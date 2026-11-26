import { useInfiniteQuery } from "@tanstack/react-query";
import { Bot, ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ConnectionInput } from "../../api/dto";
import { listRuntimesPage } from "../../api/commands";
import { keys } from "../../api/query-keys";
import { usePollingInterval } from "../../polling/focus";
import { formatTime } from "../../lib/format";
import { IconButton } from "../common/IconButton";
import { EventRow, payloadRecord } from "./EventRow";
import { buildToolEventLinks } from "./EventsPanel";
import { agentInstanceID } from "./agent-identity";
import { buildAgentConversations, runtimeIsActive } from "./agent-conversations";
import { useRuntimeAgentEvents } from "./use-runtime-agent-events";
import { AgentOnlySwitch } from "./AgentOnlySwitch";
import { isAgentMessageEvent } from "./event-filters";

const MESSAGE_PAGE_SIZE = 200;

export function AgentMessagesPanel(props: { serverId: string; subjectId: string; connection: ConnectionInput; subjectActive: boolean }) {
  return <RuntimeAgentMessages key={JSON.stringify([props.serverId, props.subjectId, props.connection.base_url, props.connection.password])} {...props} />;
}

function RuntimeAgentMessages({ serverId, subjectId, connection, subjectActive }: { serverId: string; subjectId: string; connection: ConnectionInput; subjectActive: boolean }) {
  const { t, i18n } = useTranslation();
  const history = useInfiniteQuery({
    queryKey: keys.runtimes(serverId, subjectId),
    queryFn: ({ pageParam }) => listRuntimesPage(connection, subjectId, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    refetchInterval: usePollingInterval(subjectActive, 10_000),
  });
  const runtimes = history.data?.pages.flatMap((page) => page.runtimes) ?? [];
  const [selectedRuntime, setSelectedRuntime] = useState<string | null>(null);
  const runtime = runtimes.find((item) => item.id === selectedRuntime) ?? runtimes.find((item) => item.current) ?? runtimes[0];
  useEffect(() => { if (!selectedRuntime && runtime) setSelectedRuntime(runtime.id); }, [selectedRuntime, runtime?.id]);
  useEffect(() => { void history.refetch(); }, [subjectActive]);
  const [selection, setSelection] = useState({ key: "main", end: null as number | null });
  const [agentOnly, setAgentOnly] = useState(false);
  const stream = useRuntimeAgentEvents(serverId, subjectId, runtime?.id ?? "", connection, Boolean(runtime && runtimeIsActive(runtime.state)));
  const { agents, index } = useMemo(() => buildAgentConversations(stream.events, runtime?.id ?? "", runtime?.state ?? "queued", stream.context), [stream.events, stream.context, runtime?.id, runtime?.state]);
  const selected = agents.find((agent) => agent.key === selection.key) ?? agents[0];
  const messages = useMemo(() => agentOnly ? selected.events.filter(isAgentMessageEvent) : selected.events, [selected.events, agentOnly]);
  const end = Math.min(selection.end ?? messages.length, messages.length);
  const start = Math.max(0, end - MESSAGE_PAGE_SIZE);
  const shown = messages.slice(start, end);
  const links = useMemo(() => buildToolEventLinks(messages), [messages]);
  const [highlight, setHighlight] = useState<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const followLatest = useRef(true);
  useEffect(() => { setSelection({ key: "main", end: null }); followLatest.current = true; }, [runtime?.id]);
  useEffect(() => {
    if (selection.end !== null || !followLatest.current) return;
    const frame = requestAnimationFrame(() => { if (listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight; });
    return () => cancelAnimationFrame(frame);
  }, [messages.length, selected.key, selection.end]);
  useEffect(() => {
    if (!highlight) return;
    const frame = requestAnimationFrame(() => listRef.current?.querySelector(`#event-seq-${highlight}`)?.scrollIntoView({ block: "center", behavior: "smooth" }));
    const timer = setTimeout(() => setHighlight(null), 1600);
    return () => { cancelAnimationFrame(frame); clearTimeout(timer); };
  }, [highlight]);
  const jump = (seq: string) => {
    const position = messages.findIndex((event) => event.seq === seq);
    if (position < 0) return;
    setSelection({ key: selected.key, end: Math.min(messages.length, position + MESSAGE_PAGE_SIZE) });
    setHighlight(seq);
  };
  const selectAgent = (key: string) => { setSelection({ key, end: null }); setHighlight(null); followLatest.current = true; };

  return <section className="agent-messages-panel">
    <div className="agent-messages-controls">
      <label><span>{t("agentMessages.runtime")}</span><select value={runtime?.id ?? ""} disabled={!runtimes.length} onChange={(event) => { setSelectedRuntime(event.target.value); selectAgent("main"); }}>
        {!runtimes.length && <option value="">{history.isPending ? t("common.loading") : t("agentMessages.noRuntimes")}</option>}
        {runtimes.map((item) => <option key={item.id} value={item.id}>{item.id} · {t(`status.${item.state}`)} · {formatTime(item.created_at, i18n.language)}</option>)}
      </select></label>
      <AgentOnlySwitch checked={agentOnly} onChange={(checked) => { setAgentOnly(checked); selectAgent(selected.key); }} />
      {history.hasNextPage && <button className="button secondary" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>{t("runtime.loadMore")}</button>}
      <IconButton label={t("common.refresh")} disabled={stream.loading || history.isFetching} onClick={() => { void history.refetch(); void stream.refresh(); }}><RefreshCw /></IconButton>
    </div>
    {(history.isError || stream.error) && <p className="notice warning">{stream.error || String(history.error)}<button className="button secondary" onClick={() => { void history.refetch(); void stream.refresh(); }}>{t("common.retry")}</button></p>}
    {stream.loading && <div className="agent-history-progress" role="status">{t("agentMessages.loadingHistory")}</div>}
    {runtime ? <div className="agent-messages-layout">
      <nav className="agent-roster" aria-label={t("agentMessages.agents")}>
        {agents.map((agent) => <button key={agent.key} type="button" aria-pressed={selected.key === agent.key} className={`agent-roster-item ${selected.key === agent.key ? "active" : ""}`} onClick={() => selectAgent(agent.key)}>
          <span className="agent-roster-name"><Bot />{agent.key === "unknown" ? t("agentMessages.unattributed") : agent.key === "main" ? t("events.mainAgent") : agent.agent.name}</span>
          {agent.key === "main" && <span>{agent.agent.name}</span>}
          {agentInstanceID(agent.agent) && <span className="mono agent-roster-id">{agentInstanceID(agent.agent)}</span>}
          <span className="agent-roster-meta"><span className={`agent-state agent-state-${agent.state}`}>{t(`agentMessages.${agent.state}`)}</span><span>{t("agentMessages.count", { count: agentOnly ? agent.events.filter(isAgentMessageEvent).length : agent.events.length })}</span></span>
          {agent.firstSeen && <time title={agent.firstSeen}>{formatTime(agent.firstSeen, i18n.language)}</time>}
        </button>)}
      </nav>
      <div className="agent-conversation">
        <div className="agent-conversation-toolbar"><strong>{selected.key === "unknown" ? t("agentMessages.unattributed") : selected.key === "main" ? t("events.mainAgent") : selected.agent.name}</strong><span>{start + (shown.length ? 1 : 0)}–{end} / {messages.length}</span><IconButton label={t("events.previousPage")} disabled={start === 0} onClick={() => { setSelection({ key: selected.key, end: start }); if (listRef.current) listRef.current.scrollTop = 0; }}><ChevronLeft /></IconButton><IconButton label={t("events.nextPage")} disabled={end === messages.length} onClick={() => { setSelection({ key: selected.key, end: Math.min(messages.length, end + MESSAGE_PAGE_SIZE) }); if (listRef.current) listRef.current.scrollTop = 0; }}><ChevronRight /></IconButton><button className="button secondary" disabled={selection.end === null} onClick={() => { followLatest.current = true; setSelection({ key: selected.key, end: null }); }}>{t("events.latest")}</button></div>
        <div className="event-list agent-message-list" ref={listRef} onScroll={(event) => { const node = event.currentTarget; followLatest.current = node.scrollHeight - node.scrollTop - node.clientHeight < 24; }}>
          {shown.length ? shown.map((event) => {
            const id = payloadRecord(event)?.tool_call_id;
            return <EventRow key={event.seq} event={event} agentIndex={index} linkedCall={typeof id === "string" ? links.calls.get(id) : undefined} linkedResult={typeof id === "string" ? links.results.get(id) : undefined} highlighted={event.seq === highlight} onJump={jump} />;
          }) : <div className="empty-inline">{t("agentMessages.noMessages")}</div>}
        </div>
      </div>
    </div> : <div className="empty-inline">{history.isPending ? t("common.loading") : t("agentMessages.noRuntimes")}</div>}
  </section>;
}
