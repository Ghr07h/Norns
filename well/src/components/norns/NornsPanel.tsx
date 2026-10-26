import { Hash, Network, Play, RefreshCw, Save } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { getNornsStatus, getNornsTrace, getNornsVisual } from "../../api/commands";
import type { ConnectionInput } from "../../api/dto";
import { JsonViewer } from "../common/JsonViewer";
import { errorMessage } from "../../lib/format";
import { nornsViewCacheKey, readNornsViewCache, writeNornsViewCache, type NornsViewKind } from "./view-cache";
import { saveNornsView } from "./view-export";

export function NornsPanel({ serverId, kind, connection, subjectId, enabled, switcher }: { serverId: string; kind: NornsViewKind; connection: ConnectionInput; subjectId: string; enabled: boolean; switcher?: ReactNode }) {
  const { t, i18n } = useTranslation();
  const cacheKey = nornsViewCacheKey(serverId, subjectId, kind);
  const cached = readNornsViewCache(cacheKey);
  const [parameter, setParameter] = useState(cached?.parameter ?? "");
  const [data, setData] = useState<unknown>(cached?.data ?? null);
  const [revision, setRevision] = useState(cached?.revision ?? 0);
  const [sourceFilename, setSourceFilename] = useState<string | null>(cached?.sourceFilename ?? null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [capabilityDisabled, setCapabilityDisabled] = useState(false);
  const activeKey = useRef(cacheKey);
  useEffect(() => {
    activeKey.current = cacheKey;
    const next = readNornsViewCache(cacheKey);
    setParameter(next?.parameter ?? "");
    setData(next?.data ?? null);
    setRevision(next?.revision ?? 0);
    setSourceFilename(next?.sourceFilename ?? null);
    setError("");
    setNotice("");
    setLoading(false);
    setSaving(false);
    setCapabilityDisabled(false);
    return () => { if (activeKey.current === cacheKey) activeKey.current = ""; };
  }, [cacheKey]);
  if (!enabled || capabilityDisabled) return <div className="disabled-panel"><Network /><h3>{t("norns.disabled")}</h3></div>;
  const generate = async () => {
    if (kind === "trace" && !parameter.trim()) return;
    const requestKey = cacheKey;
    const requestParameter = parameter;
    setLoading(true); setError(""); setNotice("");
    try {
      const result = kind === "status"
        ? await getNornsStatus(connection, subjectId, requestParameter.trim() || null)
        : kind === "trace"
          ? await getNornsTrace(connection, subjectId, requestParameter.trim())
          : await getNornsVisual(connection, subjectId, i18n.language.toLowerCase().startsWith("zh") ? "zh" : "en");
      const nextData = result.data;
      const nextRevision = (readNornsViewCache(requestKey)?.revision ?? 0) + 1;
      writeNornsViewCache(requestKey, { parameter: requestParameter, data: nextData, revision: nextRevision, sourceFilename: result.filename });
      if (activeKey.current === requestKey) { setData(nextData); setRevision(nextRevision); setSourceFilename(result.filename); }
    } catch (cause) {
      if (activeKey.current !== requestKey) return;
      if (typeof cause === "object" && cause && "code" in cause && (cause as { code?: string }).code === "CAPABILITY_NOT_ENABLED") setCapabilityDisabled(true);
      else setError(errorMessage(cause, t));
    } finally { if (activeKey.current === requestKey) setLoading(false); }
  };
  const parameterLabel = kind === "status" ? t("norns.taskId") : t("norns.explorationId");
  const changeParameter = (value: string) => {
    setParameter(value);
    writeNornsViewCache(cacheKey, { parameter: value, data, revision, sourceFilename });
  };
  const saveView = async () => {
    if (data === null) return;
    setSaving(true); setError(""); setNotice("");
    try {
      const path = await saveNornsView(kind, data, sourceFilename);
      if (path) setNotice(t("norns.saved"));
    } catch (cause) {
      setError(errorMessage(cause, t));
    } finally {
      setSaving(false);
    }
  };
  return <div className="norns-panel">
    <div className="panel-toolbar norns-controlbar">
      {switcher}
      <div className="norns-view-actions">
        {kind !== "visual" && <label className="norns-parameter"><Hash /><input aria-label={parameterLabel} placeholder={parameterLabel} title={parameterLabel} value={parameter} onChange={(event) => changeParameter(event.target.value)} spellCheck={false} /></label>}
        <button className="button primary norns-generate" disabled={loading || kind === "trace" && !parameter.trim()} onClick={() => void generate()}>{data !== null ? <RefreshCw /> : <Play />}{data !== null ? t("norns.regenerate") : t("norns.generate")}</button>
        {data !== null && <button className="button secondary norns-save" disabled={loading || saving} onClick={() => void saveView()}><Save />{saving ? t("norns.saving") : t("norns.saveView")}</button>}
      </div>
    </div>
    <div className="norns-view-content">
      {(error || notice) && <div className="norns-view-feedback">
        {error && <p className="notice danger">{error}</p>}
        {notice && <p className="notice success">{notice}</p>}
      </div>}
      <div className="norns-view-result">
        {loading && <div className="loading-state">{t("common.loading")}</div>}
        {!loading && kind === "visual" && typeof data === "string" ? <iframe key={`${cacheKey}-${revision}`} className="visual-frame" sandbox="allow-scripts" srcDoc={data} title="Norns Visual" /> : !loading && data !== null && kind !== "visual" ? <JsonViewer value={data} /> : !loading && <div className="empty-inline">{t("common.noData")}</div>}
      </div>
    </div>
  </div>;
}
