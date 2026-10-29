import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ConnectionInput, PublicEvent } from "../../api/dto";
import { getEventUpdates, listEvents } from "../../api/commands";
import { cacheEvents, clearSubjectCache, readCachedEventsAfter, readCachedEventsBefore, readEventCacheState, readLatestCachedEvents } from "../../storage/event-db";
import { useAppFocused } from "../../polling/focus";
import { IconButton } from "../common/IconButton";
import { EventRow, payloadRecord } from "./EventRow";

export { eventAgentLabel } from "./EventRow";

export function buildToolEventLinks(events: PublicEvent[]) {
  const calls = new Map<string, PublicEvent>();
  const results = new Map<string, PublicEvent>();
  for (const event of events) {
    const id = payloadRecord(event)?.tool_call_id;
    if (typeof id !== "string" || !id) continue;
    if (event.type === "agent.tool_call") calls.set(id, event);
    if (event.type === "agent.tool_result") results.set(id, event);
  }
  return { calls, results };
}

const EVENT_WINDOW_SIZE = 200;

export function EventsPanel({ serverId, subjectId, connection, runtimeActive }: { serverId: string; subjectId: string; connection: ConnectionInput; runtimeActive: boolean }) {
  const { t } = useTranslation();
  const focused = useAppFocused();
  const [events, setEvents] = useState<PublicEvent[]>([]);
  const [filter, setFilter] = useState("");
  const [error, setError] = useState("");
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadingNewer, setLoadingNewer] = useState(false);
  const [newCount, setNewCount] = useState(0);
  const [hasOlder, setHasOlder] = useState(false);
  const [latestSeq, setLatestSeq] = useState<string | null>(null);
  const [jumpTarget, setJumpTarget] = useState<string | null>(null);
  const [highlighted, setHighlighted] = useState<string | null>(null);
  const generation = useRef(0);
  const syncing = useRef(0);
  const syncCursor = useRef<string | null>(null);
  const olderCursor = useRef<string | null>(null);
  const atLatest = useRef(true);
  const listRef = useRef<HTMLDivElement>(null);
  const highlightTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const followingLatest = useRef(true);
  const scrollToLatest = () => { const list = listRef.current; if (list) list.scrollTop = list.scrollHeight; followingLatest.current = true; setNewCount(0); };

  const refreshOlderState = useCallback(async (window: PublicEvent[]) => {
    const cached = window[0] ? await readCachedEventsBefore(serverId, subjectId, window[0].seq, 1) : [];
    setHasOlder(cached.length > 0 || olderCursor.current !== null);
  }, [serverId, subjectId]);

  const showLatestWindow = useCallback(async (active: number) => {
    const latest = await readLatestCachedEvents(serverId, subjectId, EVENT_WINDOW_SIZE);
    if (active !== generation.current) return;
    atLatest.current = true;
    setEvents(latest);
    setLatestSeq(latest.at(-1)?.seq ?? null);
    setNewCount(0);
    await refreshOlderState(latest);
    requestAnimationFrame(scrollToLatest);
  }, [refreshOlderState, serverId, subjectId]);

  const syncFromServer = useCallback(async (showError: boolean) => {
    const active = generation.current;
    if (syncing.current === active) return;
    syncing.current = active;
    try {
      if (!syncCursor.current) {
        const page = await listEvents(connection, subjectId);
        if (active !== generation.current) return;
        await cacheEvents(serverId, subjectId, page.events, { syncCursor: page.cursor, olderCursor: page.next_cursor });
        syncCursor.current = page.cursor;
        olderCursor.current = page.next_cursor;
        await showLatestWindow(active);
        return;
      }
      const updates = await getEventUpdates(connection, subjectId, syncCursor.current);
      if (!updates.has_updates || active !== generation.current) return;
      let added = 0;
      let hasMore = true;
      while (hasMore && active === generation.current && syncCursor.current) {
        const page = await listEvents(connection, subjectId, { after: syncCursor.current });
        added += page.events.length;
        hasMore = page.has_more;
        await cacheEvents(serverId, subjectId, page.events, { syncCursor: page.cursor });
        syncCursor.current = page.cursor;
        if (page.events.length) setLatestSeq(page.events.at(-1)!.seq);
      }
      if (active !== generation.current) return;
      if (atLatest.current && followingLatest.current) await showLatestWindow(active);
      else {
        atLatest.current = false;
        setNewCount((count) => count + added);
      }
    } catch (cause) {
      const code = typeof cause === "object" && cause && "code" in cause ? (cause as { code?: string }).code : undefined;
      if (code === "INVALID_CURSOR" && active === generation.current) {
        await clearSubjectCache(serverId, subjectId);
        syncCursor.current = null;
        olderCursor.current = null;
        const page = await listEvents(connection, subjectId);
        if (active !== generation.current) return;
        await cacheEvents(serverId, subjectId, page.events, { syncCursor: page.cursor, olderCursor: page.next_cursor });
        syncCursor.current = page.cursor;
        olderCursor.current = page.next_cursor;
        await showLatestWindow(active);
      } else if (showError && active === generation.current) {
        setError((cause as Error).message);
      }
    } finally {
      if (syncing.current === active) syncing.current = 0;
    }
  }, [connection.base_url, connection.password, serverId, showLatestWindow, subjectId]);

  useEffect(() => {
    generation.current += 1;
    const active = generation.current;
    setEvents([]); setError(""); setNewCount(0); setHasOlder(false); setLatestSeq(null); followingLatest.current = true; atLatest.current = true;
    void Promise.all([readEventCacheState(serverId, subjectId), readLatestCachedEvents(serverId, subjectId, EVENT_WINDOW_SIZE)]).then(async ([state, cached]) => {
      if (active !== generation.current) return;
      syncCursor.current = state?.syncCursor ?? null;
      olderCursor.current = state?.olderCursor ?? null;
      setEvents(cached);
      setLatestSeq(cached.at(-1)?.seq ?? null);
      await refreshOlderState(cached);
      await syncFromServer(true);
    });
    return () => { generation.current += 1; };
  }, [serverId, subjectId, connection.base_url, connection.password, refreshOlderState, syncFromServer]);

  useEffect(() => {
    if (!focused || !runtimeActive) return;
    const timer = setInterval(() => void syncFromServer(false), 30_000);
    return () => clearInterval(timer);
  }, [focused, runtimeActive, syncFromServer]);

  const loadOlder = async () => {
    const before = events[0]?.seq;
    const active = generation.current;
    if (!before || syncing.current === active) return;
    syncing.current = active;
    setLoadingOlder(true); setError("");
    try {
      let previous = await readCachedEventsBefore(serverId, subjectId, before, EVENT_WINDOW_SIZE);
      if (previous.length < EVENT_WINDOW_SIZE && olderCursor.current) {
        const page = await listEvents(connection, subjectId, { cursor: olderCursor.current });
        await cacheEvents(serverId, subjectId, page.events, { olderCursor: page.next_cursor });
        olderCursor.current = page.next_cursor;
        previous = await readCachedEventsBefore(serverId, subjectId, before, EVENT_WINDOW_SIZE);
      }
      if (active !== generation.current || !previous.length) return;
      atLatest.current = false;
      setEvents(previous);
      await refreshOlderState(previous);
      if (listRef.current) listRef.current.scrollTop = 0;
    } catch (cause) {
      if (active === generation.current) setError((cause as Error).message);
    } finally {
      if (syncing.current === active) syncing.current = 0;
      if (active === generation.current) setLoadingOlder(false);
    }
  };

  const loadNewer = async () => {
    const after = events.at(-1)?.seq;
    const active = generation.current;
    if (!after || syncing.current === active) return;
    syncing.current = active;
    setLoadingNewer(true); setError("");
    try {
      const next = await readCachedEventsAfter(serverId, subjectId, after, EVENT_WINDOW_SIZE);
      if (active !== generation.current || !next.length) return;
      atLatest.current = next.at(-1)?.seq === latestSeq;
      setEvents(next);
      await refreshOlderState(next);
      if (listRef.current) listRef.current.scrollTop = 0;
    } catch (cause) {
      if (active === generation.current) setError((cause as Error).message);
    } finally {
      if (syncing.current === active) syncing.current = 0;
      if (active === generation.current) setLoadingNewer(false);
    }
  };

  const showLatest = async () => { await showLatestWindow(generation.current); };
  const shown = useMemo(() => events.filter((event) => !filter || `${event.type} ${event.source} ${event.runtime_id ?? ""} ${JSON.stringify(event.payload)}`.toLowerCase().includes(filter.toLowerCase())), [events, filter]);
  const toolLinks = useMemo(() => buildToolEventLinks(events), [events]);
  const rangeStart = events[0]?.seq ?? "0";
  const rangeEnd = events.at(-1)?.seq ?? "0";
  const hasNewer = latestSeq !== null && rangeEnd !== "0" && BigInt(rangeEnd) < BigInt(latestSeq);
  useEffect(() => {
    if (!jumpTarget) return;
    if (filter) { setFilter(""); return; }
    const frame = requestAnimationFrame(() => {
      const target = listRef.current?.querySelector<HTMLElement>(`#event-seq-${jumpTarget}`);
      if (!target) return;
      target.scrollIntoView({ behavior: "smooth", block: "center" });
      setHighlighted(jumpTarget);
      setJumpTarget(null);
      if (highlightTimer.current) clearTimeout(highlightTimer.current);
      highlightTimer.current = setTimeout(() => setHighlighted(null), 1600);
    });
    return () => cancelAnimationFrame(frame);
  }, [filter, jumpTarget, shown.length]);
  useEffect(() => () => { if (highlightTimer.current) clearTimeout(highlightTimer.current); }, []);
  return <div className="events-panel"><div className="panel-toolbar events-toolbar"><label className="search-field"><Search /><input value={filter} onChange={(event) => setFilter(event.target.value)} placeholder={t("common.search")} /></label>{newCount > 0 && <button className="button secondary" onClick={() => void showLatest()}>{t("events.newEvents")} ({newCount})</button>}<div className="event-pagination"><span className="event-page-range" aria-live="polite">{t("events.range", { start: rangeStart, end: rangeEnd, total: latestSeq ?? "0" })}</span><IconButton label={t("events.previousPage")} disabled={!hasOlder || loadingOlder || loadingNewer} onClick={() => void loadOlder()}><ChevronLeft /></IconButton><IconButton label={t("events.nextPage")} disabled={!hasNewer || loadingOlder || loadingNewer} onClick={() => void loadNewer()}><ChevronRight /></IconButton></div></div>{error && <p className="notice warning">{error}</p>}<div ref={listRef} className="event-list" onScroll={(event) => { const element = event.currentTarget; followingLatest.current = element.scrollHeight - element.scrollTop - element.clientHeight < 24; }}>{shown.length ? shown.map((event) => { const payload = payloadRecord(event); const toolID = typeof payload?.tool_call_id === "string" ? payload.tool_call_id : ""; return <EventRow key={event.seq} event={event} linkedCall={toolLinks.calls.get(toolID)} linkedResult={toolLinks.results.get(toolID)} highlighted={highlighted === event.seq} onJump={setJumpTarget} />; }) : <div className="empty-inline">{t("events.empty")}</div>}</div></div>;
}
