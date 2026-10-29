import { ArrowLeft, ListFilter, ListOrdered, Plus, RefreshCw, Search, Server, Waypoints } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { IconButton } from "../../components/common/IconButton";
import { StatusBadge } from "../../components/common/StatusBadge";
import { useQueue, useServerProbe, useSubjects } from "../../api/queries";
import { keys } from "../../api/query-keys";
import { useRegistry } from "../../storage/use-registry";
import { bindRemoteIdentity, markServerConnected } from "../../storage/registry";
import { getServerPreferences, setServerPreferences } from "../../storage/preferences";
import { SubjectDetailPanel } from "../../components/subject/SubjectDetailPanel";
import { CreateSubjectDialog } from "./CreateSubjectDialog";
import { SubjectVirtualList } from "./SubjectVirtualList";
import { QueueDialog } from "./QueueDialog";

export function ServerWorkspace() {
  const { serverId = "" } = useParams();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const client = useQueryClient();
  const { servers } = useRegistry();
  const server = servers.find((item) => item.id === serverId);
  const prefs = getServerPreferences(serverId);
  const [params, setParams] = useSearchParams();
  const selected = params.get("subject") || prefs.selectedSubjectId || null;
  const [search, setSearch] = useState(prefs.search ?? "");
  const [filter, setFilter] = useState(prefs.stateFilter ?? "all");
  const [sort, setSort] = useState(prefs.sort ?? "created");
  const [createOpen, setCreateOpen] = useState(false);
  const [queueOpen, setQueueOpen] = useState(false);
  const probe = useServerProbe(server, Boolean(server));
  const capacity = probe.connectionResult.snapshot?.service.capacity;
  const subjects = useSubjects(serverId, probe.connection, probe.connectionResult.state === "online", Boolean(capacity && capacity.using + capacity.queuing > 0));
  const queue = useQueue(serverId, probe.connection, probe.connectionResult.state === "online" && queueOpen);
  useEffect(() => {
    if (!server || probe.connectionResult.state !== "online") return;
    if (probe.connectionResult.currentRemoteId && !server.remote_server_id) {
      bindRemoteIdentity(server.id, probe.connectionResult.currentRemoteId);
    } else if (probe.dataUpdatedAt) markServerConnected(server.id);
  }, [probe.connectionResult.state, probe.connectionResult.currentRemoteId, probe.dataUpdatedAt, server?.id, server?.remote_server_id]);
  const shown = useMemo(() => (subjects.data ?? []).filter((subject) => {
    const query = search.trim().toLowerCase();
    const matchesSearch = !query || `${subject.name ?? ""} ${subject.id}`.toLowerCase().includes(query);
    const matchesFilter = filter === "all" || filter === "active" && subject.state === "using" || filter === "queued" && subject.state === "queuing" || subject.state === filter;
    return matchesSearch && matchesFilter;
  }).sort((a, b) => sort === "name" ? (a.name ?? "").localeCompare(b.name ?? "") || a.id.localeCompare(b.id) : 0), [subjects.data, search, filter, sort]);
  const selectedSubject = subjects.data?.find((subject) => subject.id === selected);
  useEffect(() => {
    if (!selected || !subjects.data || subjects.data.some((subject) => subject.id === selected)) return;
    setParams({});
    setServerPreferences(serverId, { selectedSubjectId: undefined });
  }, [selected, serverId, setParams, subjects.data]);
  if (!server) return <div className="fatal-error"><h1>{t("workspace.serverNotFound")}</h1><button className="button secondary" onClick={() => navigate("/servers")}>{t("common.back")}</button></div>;
  const select = (id: string) => { const next = new URLSearchParams(params); next.set("subject", id); if (!next.get("panel") && prefs.activePanel) next.set("panel", prefs.activePanel); setParams(next); setServerPreferences(serverId, { selectedSubjectId: id }); };
  const refresh = async () => { await Promise.all([client.invalidateQueries({ queryKey: keys.probe(serverId) }), client.invalidateQueries({ queryKey: keys.subjects(serverId) }), client.invalidateQueries({ queryKey: keys.queue(serverId) })]); };
  const info = probe.connectionResult.snapshot?.service;
  if (!probe.connection || probe.connectionResult.state !== "online" || !info) return <div className="workspace-gate"><div className="workspace-gate-info"><div className="workspace-gate-symbol"><Waypoints /></div><div className="workspace-gate-copy"><div><h1>{server.name}</h1><StatusBadge value={probe.connectionResult.state} /></div><p className="mono">{server.base_url}</p></div></div><div className="workspace-gate-actions"><button className="button secondary" onClick={() => navigate("/servers")}><ArrowLeft />{t("common.back")}</button><button className="button primary" disabled={!probe.connection} onClick={() => void probe.refetch()}><RefreshCw />{t("common.retry")}</button></div></div>;
  return <div className="server-workspace"><header className="server-topbar"><div className="topbar-identity"><IconButton label={t("common.back")} onClick={() => navigate("/servers")}><ArrowLeft /></IconButton><Server /><div><h1>{server.name}</h1><span className="mono">{server.base_url}</span></div></div><div className="capacity-strip"><StatusBadge value="online" /><span className="capacity-metric"><strong>{info.capacity.using}</strong> / {info.capacity.concurrency} {t("status.using")}</span><span className="capacity-metric"><strong>{info.capacity.queuing}</strong> {t("status.queuing")}</span><button className="queue-trigger" onClick={() => setQueueOpen(true)}><ListOrdered /><strong>{info.capacity.queuing}</strong> {t("workspace.queue")}</button></div><div className="toolbar-actions"><button className="queue-toolbar" title={t("workspace.queue")} aria-label={t("workspace.queue")} onClick={() => setQueueOpen(true)}><ListOrdered /><span>{info.capacity.queuing}</span></button><IconButton label={t("workspace.refreshAll")} onClick={() => void refresh()}><RefreshCw /></IconButton><button className="button primary" onClick={() => setCreateOpen(true)}><Plus />{t("workspace.create")}</button></div></header>
    <div className="workspace-main"><aside className="subject-sidebar"><div className="sidebar-heading"><h2>{t("workspace.subjects")}</h2><span>{t("workspace.subjectTotal", { count: subjects.data?.length ?? 0 })}</span></div><div className="subject-filters"><label className="search-field"><Search /><input value={search} onChange={(event) => { setSearch(event.target.value); setServerPreferences(serverId, { search: event.target.value }); }} placeholder={t("workspace.filter")} /></label><label className="select-field"><ListFilter /><select value={filter} onChange={(event) => { setFilter(event.target.value); setServerPreferences(serverId, { stateFilter: event.target.value }); }}><option value="all">{t("common.all")}</option><option value="active">{t("status.using")}</option><option value="queued">{t("status.queuing")}</option><option value="idle">{t("status.idle")}</option></select></label><label className="select-field sort-field"><select aria-label={t("workspace.sort")} value={sort} onChange={(event) => { setSort(event.target.value); setServerPreferences(serverId, { sort: event.target.value }); }}><option value="created">{t("workspace.sortCreated")}</option><option value="name">{t("workspace.sortName")}</option></select></label></div>{selected && !shown.some((subject) => subject.id === selected) && <p className="filter-note">{t("workspace.selectedFiltered")}</p>}{subjects.isError && <p className="notice warning">{String(subjects.error)}</p>}{shown.length ? <SubjectVirtualList subjects={shown} selected={selected} onSelect={select} /> : <div className="empty-inline"><span>{t("workspace.noSubjects")}</span>{(search || filter !== "all") && <button className="button secondary" onClick={() => { setSearch(""); setFilter("all"); setServerPreferences(serverId, { search: "", stateFilter: "all" }); }}>{t("workspace.clearFilters")}</button>}{!subjects.data?.length && <button className="button primary" onClick={() => setCreateOpen(true)}><Plus />{t("workspace.create")}</button>}</div>}</aside><main className="detail-region"><SubjectDetailPanel serverId={serverId} subjectId={selected} subjectActive={Boolean(selectedSubject && selectedSubject.state !== "idle")} connection={probe.connection} service={info} initialPanel={params.get("panel") || prefs.activePanel} onPanelChange={(panel) => { const next = new URLSearchParams(params); next.set("panel", panel); if (selected) next.set("subject", selected); setParams(next); setServerPreferences(serverId, { activePanel: panel }); }} /></main></div>
    <CreateSubjectDialog open={createOpen} connection={probe.connection} onClose={() => setCreateOpen(false)} onCreated={(id) => { select(id); void refresh(); }} />
    <QueueDialog open={queueOpen} entries={queue.data ?? []} loading={queue.isPending} error={queue.error} onClose={() => setQueueOpen(false)} onSelect={(id) => { select(id); setQueueOpen(false); }} />
  </div>;
}
