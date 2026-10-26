import Dexie, { type Table } from "dexie";
import type { PublicEvent } from "../api/dto";

export interface EventRow extends PublicEvent {
  serverId: string;
  subjectId: string;
  seqSortKey: string;
  approximateBytes: number;
  lastAccessed: number;
}

export interface CacheScope {
  id: string;
  serverId: string;
  subjectId: string;
  approximateBytes: number;
  eventCount?: number;
  lastAccessed: number;
  syncCursor?: string | null;
  olderCursor?: string | null;
}

class EventDatabase extends Dexie {
  events!: Table<EventRow, [string, string, string]>;
  scopes!: Table<CacheScope, string>;

  constructor() {
    super("well-event-cache");
    this.version(1).stores({
      events: "&[serverId+subjectId+seqSortKey], [serverId+subjectId], [serverId+subjectId+seqSortKey], serverId, lastAccessed",
      scopes: "&id, serverId, lastAccessed",
    });
  }
}

export const eventDb = new EventDatabase();

export function seqSortKey(seq: string): string {
  if (!/^(0|[1-9]\d{0,19})$/.test(seq) || BigInt(seq) > 18_446_744_073_709_551_615n) throw new Error("invalid_event_seq");
  return seq.padStart(20, "0");
}

function scopeId(serverId: string, subjectId: string): string {
  return `${serverId}\u0000${subjectId}`;
}

function approximateSize(event: PublicEvent): number {
  return new TextEncoder().encode(JSON.stringify(event)).byteLength;
}

export interface EventCacheCursors {
  syncCursor?: string | null;
  olderCursor?: string | null;
}

const scopeRange = (serverId: string, subjectId: string) => ({
  lower: [serverId, subjectId, ""] as [string, string, string],
  upper: [serverId, subjectId, "\uffff"] as [string, string, string],
});

function publicEvent(row: EventRow): PublicEvent {
  const { serverId: _server, subjectId: _subject, seqSortKey: _key, approximateBytes: _bytes, lastAccessed: _accessed, ...event } = row;
  return event;
}

export async function cacheEvents(serverId: string, subjectId: string, events: PublicEvent[], cursors: EventCacheCursors = {}): Promise<void> {
  if (!events.length && cursors.syncCursor === undefined && cursors.olderCursor === undefined) return;
  const now = Date.now();
  await eventDb.transaction("rw", eventDb.events, eventDb.scopes, async () => {
    const rows = events.map((event) => ({ ...event, serverId, subjectId, seqSortKey: seqSortKey(event.seq), approximateBytes: approximateSize(event), lastAccessed: now }));
    const keys = rows.map((row) => [row.serverId, row.subjectId, row.seqSortKey] as [string, string, string]);
    const existing = keys.length ? await eventDb.events.bulkGet(keys) : [];
    if (rows.length) await eventDb.events.bulkPut(rows);
    const id = scopeId(serverId, subjectId);
    const previous = await eventDb.scopes.get(id);
    const existingCount = previous?.eventCount ?? await eventDb.events.where("[serverId+subjectId]").equals([serverId, subjectId]).count() - rows.filter((_, index) => !existing[index]).length;
    let bytes = previous?.approximateBytes ?? 0;
    rows.forEach((row, index) => {
      bytes += row.approximateBytes - (existing[index]?.approximateBytes ?? 0);
    });
    const scope: CacheScope = {
      id,
      serverId,
      subjectId,
      approximateBytes: Math.max(0, bytes),
      eventCount: existingCount + rows.filter((_, index) => !existing[index]).length,
      lastAccessed: now,
      syncCursor: cursors.syncCursor === undefined ? previous?.syncCursor : cursors.syncCursor,
      olderCursor: cursors.olderCursor === undefined ? previous?.olderCursor : cursors.olderCursor,
    };
    await eventDb.scopes.put(scope);
  });
  await pruneGlobalCache(scopeId(serverId, subjectId));
}

export async function readEventCacheState(serverId: string, subjectId: string): Promise<CacheScope | undefined> {
  return eventDb.transaction("rw", eventDb.scopes, async () => {
    const scope = await eventDb.scopes.get(scopeId(serverId, subjectId));
    if (scope) {
      scope.lastAccessed = Date.now();
      await eventDb.scopes.put(scope);
    }
    return scope;
  });
}

export async function readLatestCachedEvents(serverId: string, subjectId: string, limit: number): Promise<PublicEvent[]> {
  const range = scopeRange(serverId, subjectId);
  const rows = await eventDb.events.where("[serverId+subjectId+seqSortKey]").between(range.lower, range.upper, true, true).reverse().limit(limit).toArray();
  return rows.reverse().map(publicEvent);
}

export async function readCachedEventsBefore(serverId: string, subjectId: string, before: string, limit: number): Promise<PublicEvent[]> {
  const range = scopeRange(serverId, subjectId);
  const upper = [serverId, subjectId, seqSortKey(before)] as [string, string, string];
  const rows = await eventDb.events.where("[serverId+subjectId+seqSortKey]").between(range.lower, upper, true, false).reverse().limit(limit).toArray();
  return rows.reverse().map(publicEvent);
}

export async function readCachedEventsAfter(serverId: string, subjectId: string, after: string, limit: number): Promise<PublicEvent[]> {
  const range = scopeRange(serverId, subjectId);
  const lower = [serverId, subjectId, seqSortKey(after)] as [string, string, string];
  const rows = await eventDb.events.where("[serverId+subjectId+seqSortKey]").between(lower, range.upper, false, true).limit(limit).toArray();
  return rows.map(publicEvent);
}

export async function clearSubjectCache(serverId: string, subjectId: string): Promise<void> {
  await eventDb.transaction("rw", eventDb.events, eventDb.scopes, async () => {
    await eventDb.events.where("[serverId+subjectId]").equals([serverId, subjectId]).delete();
    await eventDb.scopes.delete(scopeId(serverId, subjectId));
  });
}

async function pruneGlobalCache(activeScope: string): Promise<void> {
  const limit = 256 * 1024 * 1024;
  let scopes = await eventDb.scopes.orderBy("lastAccessed").toArray();
  let total = scopes.reduce((sum, scope) => sum + scope.approximateBytes, 0);
  for (const scope of scopes) {
    if (total <= limit) break;
    if (scope.id === activeScope) continue;
    await eventDb.events.where("[serverId+subjectId]").equals([scope.serverId, scope.subjectId]).delete();
    await eventDb.scopes.delete(scope.id);
    total -= scope.approximateBytes;
  }
  scopes = [];
}

export async function clearServerCache(serverId: string): Promise<void> {
  await eventDb.transaction("rw", eventDb.events, eventDb.scopes, async () => {
    await eventDb.events.where("serverId").equals(serverId).delete();
    await eventDb.scopes.where("serverId").equals(serverId).delete();
  });
}

export async function clearAllCaches(): Promise<void> {
  await eventDb.transaction("rw", eventDb.events, eventDb.scopes, async () => {
    await eventDb.events.clear();
    await eventDb.scopes.clear();
  });
}

export async function cacheSummary(): Promise<{ events: number; bytes: number }> {
  const scopes = await eventDb.scopes.toArray();
  const events = scopes.some((scope) => scope.eventCount === undefined) ? await eventDb.events.count() : scopes.reduce((sum, scope) => sum + (scope.eventCount ?? 0), 0);
  return { events, bytes: scopes.reduce((sum, scope) => sum + scope.approximateBytes, 0) };
}
