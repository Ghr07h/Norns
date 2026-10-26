import { Database, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { getVersion } from "@tauri-apps/api/app";
import { Dialog } from "../../components/common/Dialog";
import { cacheSummary, clearAllCaches } from "../../storage/event-db";
import { formatBytes } from "../../lib/format";

export function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [summary, setSummary] = useState({ events: 0, bytes: 0 });
  const [version, setVersion] = useState("0.1.0");
  useEffect(() => { if (open) { void cacheSummary().then(setSummary); void getVersion().then(setVersion).catch(() => undefined); } }, [open]);
  const clear = async () => { if (!confirm(t("settings.clearConfirm"))) return; await clearAllCaches(); client.clear(); setSummary({ events: 0, bytes: 0 }); };
  return <Dialog open={open} title={t("settings.title")} onClose={onClose} footer={<button className="button secondary" onClick={onClose}>{t("common.close")}</button>}>
    <section className="settings-section"><div className="settings-icon"><Database /></div><div><h3>{t("settings.cache")}</h3><p>{t("settings.cacheEvents", { count: summary.events })} · {t("settings.cacheSize", { size: formatBytes(summary.bytes) })}</p></div><button className="button danger" onClick={() => void clear()}><Trash2 />{t("settings.clearAll")}</button></section>
    <section className="settings-section"><div><h3>{t("settings.version")}</h3><p className="mono">{version}</p></div></section>
  </Dialog>;
}
