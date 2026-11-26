import { useCallback, useEffect, useRef, useState } from "react";
import type { ConnectionInput, PublicEvent } from "../../api/dto";
import { getEventUpdates, listEvents } from "../../api/commands";
import { useAppFocused } from "../../polling/focus";
import { cacheEvents, clearSubjectCache, readCachedEventContext, readCachedRuntimeAgentEvents, readEventCacheState } from "../../storage/event-db";
import { agentContextEvent } from "./agent-identity";

export function compareEventSequence(a: PublicEvent, b: PublicEvent): number {
  const left = BigInt(a.seq), right = BigInt(b.seq);
  return left < right ? -1 : left > right ? 1 : 0;
}

// Backfill the shared Subject cache, then retain only this Runtime's agent
// events in memory. Cursors cover all event types, including filtered ones.
export function useRuntimeAgentEvents(serverId: string, subjectId: string, runtimeId: string, connection: ConnectionInput, active: boolean) {
  const focused = useAppFocused();
  const scope = JSON.stringify([serverId, subjectId, runtimeId, connection.base_url, connection.password]);
  const [state, setState] = useState<{ scope: string; events: PublicEvent[]; context: PublicEvent[]; loading: boolean; error: string }>({ scope, events: [], context: [], loading: Boolean(runtimeId), error: "" });
  const refreshRef = useRef<() => Promise<void>>(async () => undefined);
  useEffect(() => {
    let cancelled = false;
    let busy = false;
    let initialized = false;
    let refreshPending = false;
    const records = new Map<string, PublicEvent>();
    const context = new Map<string, PublicEvent>();
    const publish = (loading: boolean, error = "") => {
      if (!cancelled) setState({ scope, events: [...records.values()].sort(compareEventSequence), context: [...context.values()], loading, error });
    };
    const add = (events: PublicEvent[], contextOnly = false) => {
      for (const event of events) {
        if (!contextOnly && event.runtime_id === runtimeId && event.type.startsWith("agent.")) records.set(event.seq, event);
        const metadata = agentContextEvent(event);
        if (metadata) context.set(JSON.stringify([metadata.runtime_id, metadata.type, metadata.payload]), metadata);
      }
    };
    const sync = async (): Promise<void> => {
      if (cancelled || !initialized || !runtimeId) return;
      if (busy) { refreshPending = true; return; }
      busy = true;
      publish(true);
      try {
        // Restart once if a cursor no longer belongs to the remote event log.
        for (let attempt = 0; attempt < 2; attempt += 1) {
          try {
            const cached = await readEventCacheState(serverId, subjectId);
            if (cancelled) return;
            let after = cached?.syncCursor ?? null;
            let older = cached?.olderCursor ?? null;
            if (!after) {
              const page = await listEvents(connection, subjectId, { limit: 1000 });
              if (cancelled) return;
              await cacheEvents(serverId, subjectId, page.events, { syncCursor: page.cursor, olderCursor: page.next_cursor });
              add(page.events);
              older = page.next_cursor;
              publish(true);
            } else if ((await getEventUpdates(connection, subjectId, after)).has_updates) {
              let more = true;
              const seen = new Set<string>();
              while (more && after && !cancelled) {
                if (seen.has(after)) throw new Error("Repeated event cursor");
                seen.add(after);
                const page = await listEvents(connection, subjectId, { after, limit: 1000 });
                if (cancelled) return;
                await cacheEvents(serverId, subjectId, page.events, { syncCursor: page.cursor });
                add(page.events);
                after = page.cursor;
                more = page.has_more;
                publish(true);
              }
            }
            const seen = new Set<string>();
            while (older && !cancelled) {
              if (seen.has(older)) throw new Error("Repeated history cursor");
              seen.add(older);
              const page = await listEvents(connection, subjectId, { cursor: older, limit: 1000 });
              if (cancelled) return;
              await cacheEvents(serverId, subjectId, page.events, { olderCursor: page.next_cursor });
              add(page.events);
              older = page.next_cursor;
              publish(true);
            }
            break;
          } catch (cause) {
            if (cancelled) return;
            const code = typeof cause === "object" && cause && "code" in cause ? cause.code : undefined;
            if (code !== "INVALID_CURSOR" || attempt !== 0) throw cause;
            await clearSubjectCache(serverId, subjectId);
            records.clear();
            context.clear();
            publish(true);
          }
        }
        publish(false);
      } catch (cause) {
        publish(false, cause instanceof Error ? cause.message : String((cause as { message?: string })?.message ?? cause));
      } finally {
        busy = false;
        if (refreshPending && !cancelled) { refreshPending = false; void sync(); }
      }
    };
    refreshRef.current = sync;
    publish(Boolean(runtimeId));
    if (runtimeId) void Promise.all([readCachedRuntimeAgentEvents(serverId, subjectId, runtimeId), readCachedEventContext(serverId, subjectId, agentContextEvent)]).then(([events, cachedContext]) => {
      if (cancelled) return;
      add(events);
      add(cachedContext, true);
      initialized = true;
      return sync();
    }).catch((cause) => publish(false, String(cause)));
    return () => { cancelled = true; };
  }, [scope, serverId, subjectId, runtimeId, connection.base_url, connection.password]);

  useEffect(() => {
    if (!focused || !runtimeId) return;
    // Fetch the final output when the Runtime ends, before stopping polling.
    void refreshRef.current();
    if (!active) return;
    const timer = setInterval(() => void refreshRef.current(), 30_000);
    return () => clearInterval(timer);
  }, [focused, active, runtimeId]);
  const refresh = useCallback(() => refreshRef.current(), []);
  return { ...(state.scope === scope ? state : { events: [], context: [], loading: Boolean(runtimeId), error: "" }), refresh };
}
