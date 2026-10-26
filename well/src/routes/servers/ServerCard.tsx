import { Database, ExternalLink, KeyRound, LogOut, Pencil, RefreshCw, Server, Trash2 } from "lucide-react";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { IconButton } from "../../components/common/IconButton";
import { StatusBadge } from "../../components/common/StatusBadge";
import { formatTime } from "../../lib/format";
import { bindRemoteIdentity, markServerConnected, passwordFor } from "../../storage/registry";
import type { ServerRecord } from "../../storage/registry-schema";
import { useServerProbe } from "../../api/queries";

export function ServerCard({ server, onEdit, onRemove, onClearCache, onDisconnect, onIdentityChange }: { server: ServerRecord; onEdit: () => void; onRemove: () => void; onClearCache: () => void; onDisconnect: () => void; onIdentityChange: (remoteId: string) => void }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const probe = useServerProbe(server);
  const result = probe.connectionResult;
  useEffect(() => {
    if (result.state === "online" && result.currentRemoteId && !server.remote_server_id) {
      try { bindRemoteIdentity(server.id, result.currentRemoteId); } catch { /* next probe exposes duplicate identity */ }
    } else if (result.state === "online" && probe.dataUpdatedAt) {
      markServerConnected(server.id);
    }
  }, [probe.dataUpdatedAt, result.state, result.currentRemoteId, server.id, server.remote_server_id]);
  const info = result.snapshot?.service;
  const liveCapacity = result.state === "online" ? info?.capacity : undefined;
  const needsPassword = result.state === "unknown" || result.state === "unauthorized";
  return <article className="server-row">
    <div className="server-symbol"><Server /></div>
    <div className="server-main"><div className="server-title-line"><h2>{server.name}</h2></div><p className="server-url mono">{server.base_url}</p><div className="server-metadata"><span>{info ? `nornsd ${info.version} · API ${info.api_version}` : "nornsd"}</span><span>{info ? `${info.runtime_profile.agent} ${info.runtime_profile.agent_version}` : "-"}</span><span>{t("servers.lastConnected")}: {server.last_connected_at ? formatTime(server.last_connected_at, i18n.language) : t("servers.never")}</span></div></div>
    <div className="server-status"><StatusBadge value={result.state} /></div>
    <div className="server-capacity">{liveCapacity ? <><span><strong>{liveCapacity.using}/{liveCapacity.concurrency}</strong><small>{t("servers.capacity")}</small></span><span><strong>{liveCapacity.queuing}</strong><small>{t("status.queuing")}</small></span></> : <span className="capacity-unavailable">-</span>}</div>
    <div className="server-feature"><Database /><span>{info?.capabilities.includes("norns_views") ? t("servers.norns") : "Agent"}</span></div>
    <div className="row-actions"><IconButton label={t("common.refresh")} disabled={!probe.connection} onClick={() => void probe.refetch()}><RefreshCw /></IconButton><IconButton label={t("servers.clearCache")} onClick={onClearCache}><Database /></IconButton>{!server.remember_password && passwordFor(server) && <IconButton label={t("servers.disconnect")} onClick={onDisconnect}><LogOut /></IconButton>}<IconButton label={t("common.edit")} onClick={onEdit}><Pencil /></IconButton><IconButton label={t("common.remove")} onClick={onRemove}><Trash2 /></IconButton>{result.state === "identity_changed" && result.currentRemoteId ? <button className="button warning" onClick={() => onIdentityChange(result.currentRemoteId!)}>{t("servers.confirmIdentity")}</button> : needsPassword ? <button className="button primary" onClick={onEdit}><KeyRound />{t("servers.connectPassword")}</button> : <button className="button primary" disabled={result.state !== "online"} onClick={() => navigate(`/servers/${server.id}`)}>{t("servers.enter")}<ExternalLink /></button>}</div>
  </article>;
}
