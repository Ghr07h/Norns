import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../components/common/Dialog";
import type { ServerRecord } from "../../storage/registry-schema";
import { passwordFor, saveServer } from "../../storage/registry";
import { errorMessage } from "../../lib/format";

export function ServerDialog({ open, server, onClose, onSaved }: { open: boolean; server?: ServerRecord; onClose: () => void; onSaved: (record: ServerRecord, urlChanged: boolean) => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("http://127.0.0.1:28081");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!open) return;
    setName(server?.name ?? ""); setUrl(server?.base_url ?? "http://127.0.0.1:28081"); setPassword(server ? passwordFor(server) ?? "" : ""); setRemember(server?.remember_password ?? true); setError("");
  }, [open, server]);
  const submit = async () => {
    setBusy(true); setError("");
    try { const result = await saveServer({ id: server?.id, name, baseUrl: url, password, rememberPassword: remember }); onSaved(result.server, result.urlChanged); onClose(); }
    catch (cause) { setError(errorMessage(cause, t)); }
    finally { setBusy(false); }
  };
  return <Dialog open={open} title={server ? t("servers.editTitle") : t("servers.addTitle")} onClose={onClose} footer={<><button className="button secondary" onClick={onClose}>{t("common.cancel")}</button><button className="button primary" disabled={busy} onClick={() => void submit()}>{t("common.save")}</button></>}>
    <div className="form-grid"><label><span>{t("servers.name")}</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} /></label><label><span>{t("servers.url")}</span><input value={url} onChange={(event) => setUrl(event.target.value)} spellCheck={false} /></label><label><span>{t("servers.password")}</span><input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" /></label><label className="check-row"><input type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} /><span>{t("servers.remember")}</span></label>{error && <p className="form-error" role="alert">{error}</p>}</div>
  </Dialog>;
}
