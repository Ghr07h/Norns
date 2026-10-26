import { Plus, Settings2, Waypoints } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Dialog } from "../../components/common/Dialog";
import { IconButton } from "../../components/common/IconButton";
import { errorMessage } from "../../lib/format";
import { clearServerCache } from "../../storage/event-db";
import { confirmRemoteIdentity, disconnectServer, ensureRemoteIdentityAvailable, removeServer } from "../../storage/registry";
import type { ServerRecord } from "../../storage/registry-schema";
import { removeServerPreferences } from "../../storage/preferences";
import { useRegistry } from "../../storage/use-registry";
import { ServerCard } from "./ServerCard";
import { ServerDialog } from "./ServerDialog";
import { SettingsDialog } from "./SettingsDialog";

export function ServersPage() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { servers, errors } = useRegistry();
  const [dialog, setDialog] = useState<ServerRecord | null | undefined>(undefined);
  const [settings, setSettings] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<ServerRecord | null>(null);
  const [identityTarget, setIdentityTarget] = useState<{ server: ServerRecord; remoteId: string } | null>(null);
  const [identityBusy, setIdentityBusy] = useState(false);
  const [identityError, setIdentityError] = useState("");
  const [notice, setNotice] = useState("");
  const sorted = useMemo(() => [...servers].sort((a, b) => a.created_at.localeCompare(b.created_at)), [servers]);
  useEffect(() => {
    if (!notice) return;
    const timeout = window.setTimeout(() => setNotice(""), 3_000);
    return () => window.clearTimeout(timeout);
  }, [notice]);
  const clearQueries = (serverId: string) => client.removeQueries({ queryKey: ["server", serverId] });
  const remove = async () => {
    if (!removeTarget) return;
    const server = removeTarget;
    setRemoveTarget(null);
    removeServer(server.id);
    removeServerPreferences(server.id);
    clearQueries(server.id);
    await clearServerCache(server.id);
    setNotice(t("servers.removed", { name: server.name }));
  };
  const closeIdentityDialog = () => {
    if (identityBusy) return;
    setIdentityTarget(null);
    setIdentityError("");
  };
  const rebind = async () => {
    if (!identityTarget || identityBusy) return;
    const { server, remoteId } = identityTarget;
    setIdentityBusy(true);
    setIdentityError("");
    try {
      ensureRemoteIdentityAvailable(server.id, remoteId);
      await clearServerCache(server.id);
      removeServerPreferences(server.id);
      clearQueries(server.id);
      confirmRemoteIdentity(server.id, remoteId);
      setIdentityTarget(null);
      setNotice(t("servers.identityRebound", { name: server.name }));
    } catch (cause) {
      setIdentityError(errorMessage(cause, t));
    } finally {
      setIdentityBusy(false);
    }
  };
  const clear = async (server: ServerRecord) => { await clearServerCache(server.id); clearQueries(server.id); setNotice(t("servers.cacheCleared", { name: server.name })); };
  const disconnect = (serverId: string) => { disconnectServer(serverId); clearQueries(serverId); };
  return <div className="app-page servers-page"><header className="app-toolbar"><div className="brand"><Waypoints /><h1>Well</h1></div><div className="toolbar-actions"><span className="toolbar-count">{servers.length} {t("servers.title")}</span><IconButton label={t("common.settings")} onClick={() => setSettings(true)}><Settings2 /></IconButton><button className="button primary" onClick={() => setDialog(null)}><Plus />{t("servers.add")}</button></div></header>
    <main className="servers-content">{notice && <div className="notice success" role="status">{notice}</div>}{errors.map((error) => <div className="notice warning" key={error}>{t(`errors.${error}`, { defaultValue: error })}</div>)}{sorted.length ? <div className="server-list">{sorted.map((server) => <ServerCard key={server.id} server={server} onEdit={() => setDialog(server)} onRemove={() => setRemoveTarget(server)} onClearCache={() => void clear(server)} onDisconnect={() => disconnect(server.id)} onIdentityChange={(remoteId) => { setIdentityError(""); setIdentityTarget({ server, remoteId }); }} />)}</div> : <div className="empty-state"><Waypoints /><h2>{t("servers.empty")}</h2><p>{t("servers.emptyHint")}</p><button className="button primary" onClick={() => setDialog(null)}><Plus />{t("servers.add")}</button></div>}</main>
    <ServerDialog open={dialog !== undefined} server={dialog ?? undefined} onClose={() => setDialog(undefined)} onSaved={(saved, urlChanged) => { if (urlChanged && dialog) { void clearServerCache(dialog.id); removeServerPreferences(dialog.id); } clearQueries(saved.id); }} />
    <Dialog open={removeTarget !== null} title={t("servers.removeTitle")} onClose={() => setRemoveTarget(null)} footer={<><button className="button secondary" onClick={() => setRemoveTarget(null)}>{t("common.cancel")}</button><button className="button danger" onClick={() => void remove()}>{t("common.remove")}</button></>}>
      <p>{t("servers.removeBody")}</p>
    </Dialog>
    <Dialog open={identityTarget !== null} title={t("servers.identityTitle")} onClose={closeIdentityDialog} footer={<><button className="button secondary" disabled={identityBusy} onClick={closeIdentityDialog}>{t("common.cancel")}</button><button className="button warning" disabled={identityBusy} onClick={() => void rebind()}>{identityBusy ? t("common.loading") : t("servers.confirmIdentity")}</button></>}>
      <p>{t("servers.identityBody")}</p>
      {identityError && <p className="form-error" role="alert">{identityError}</p>}
    </Dialog>
    <SettingsDialog open={settings} onClose={() => setSettings(false)} />
  </div>;
}
