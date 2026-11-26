import { Activity, ListTree, MessagesSquare, ScrollText, Workflow } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ConnectionInput, ServiceInfo } from "../../api/dto";
import { useCurrentRuntime, useSubject } from "../../api/queries";
import { OverviewPanel } from "./OverviewPanel";
import { RuntimePanel } from "../runtime/RuntimePanel";
import { AgentMessagesPanel } from "../events/AgentMessagesPanel";
import { EventsPanel } from "../events/EventsPanel";
import { NornsViewsPanel, type NornsViewKind } from "../norns/NornsViewsPanel";

type Tab = "overview" | "runtime" | "events" | "agent-messages" | "norns";

function tabFrom(value: string | undefined): Tab {
  if (value && ["status", "trace", "visual"].includes(value)) return "norns";
  if (value === "history") return "runtime";
  return value && ["overview", "runtime", "events", "agent-messages", "norns"].includes(value) ? value as Tab : "overview";
}

function nornsKindFrom(value: string | undefined): NornsViewKind {
  return value === "trace" || value === "status" ? value : "visual";
}

export function SubjectDetailPanel({ serverId, subjectId, subjectActive, connection, initialPanel, onPanelChange }: { serverId: string; subjectId: string | null; subjectActive: boolean; connection: ConnectionInput; service: ServiceInfo; initialPanel?: string; onPanelChange: (panel: Tab) => void }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>(tabFrom(initialPanel));
  useEffect(() => { setTab(tabFrom(initialPanel)); }, [subjectId, initialPanel]);
  const detail = useSubject(serverId, subjectId, connection);
  const current = useCurrentRuntime(serverId, subjectId, connection, tab === "runtime");
  const tabs = useMemo(() => [
    ["overview", t("subject.overview"), ListTree], ["runtime", t("subject.runtime"), Activity], ["agent-messages", t("subject.agentMessages"), MessagesSquare], ["events", t("subject.events"), ScrollText], ["norns", t("subject.nornsView"), Workflow],
  ] as const, [t]);
  if (!subjectId) return <div className="subject-empty"><ListTree /><h2>{t("subject.noSelection")}</h2></div>;
  if (detail.isError && !detail.data) return <div className="subject-empty"><ListTree /><h2>{t("subject.loadFailed")}</h2><p>{String(detail.error)}</p><button className="button secondary" onClick={() => void detail.refetch()}>{t("common.retry")}</button></div>;
  if (detail.isPending || !detail.data) return <div className="subject-skeleton"><div /><div /><div /></div>;
  return <div className="subject-detail"><header className="subject-header"><div><h2>{detail.data.name || t("subject.unnamed")}</h2><p className="mono">{detail.data.id}</p></div></header><nav className="subject-tabs" aria-label="Subject panels">{tabs.map(([value, label, Icon]) => <button key={value} className={tab === value ? "active" : ""} onClick={() => { setTab(value); onPanelChange(value); }} ><Icon />{label}</button>)}</nav><div className="subject-panel">{tab === "overview" && <OverviewPanel subject={detail.data} />}{tab === "runtime" && (current.isPending ? <div className="loading-state">{t("common.loading")}</div> : <RuntimePanel serverId={serverId} connection={connection} subject={detail.data} runtime={current.data?.runtime ?? null} />)}{tab === "events" && <EventsPanel serverId={serverId} subjectId={subjectId} connection={connection} runtimeActive={subjectActive} />}{tab === "agent-messages" && <AgentMessagesPanel serverId={serverId} subjectId={subjectId} connection={connection} subjectActive={subjectActive} />}{tab === "norns" && <NornsViewsPanel serverId={serverId} connection={connection} subjectId={subjectId} enabled initialKind={nornsKindFrom(initialPanel)} />}</div></div>;
}
