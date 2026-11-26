import { Hash, Play, RefreshCw, Save } from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { parse as parseYaml, stringify as stringifyYaml } from "yaml";
import { getNornsEntity, getNornsStatus, getNornsTrace, getNornsVisual, type ViewFormat } from "../../api/commands";
import type { ConnectionInput } from "../../api/dto";
import { JsonViewer } from "../common/JsonViewer";
import { CopyButton } from "../common/CopyButton";
import { errorMessage } from "../../lib/format";
import { nornsViewCacheKey, readNornsViewCache, writeNornsViewCache, type NornsViewKind } from "./view-cache";
import { saveNornsView } from "./view-export";

function ViewPreview({ data, format }: { data: unknown; format: ViewFormat }) {
  const { t } = useTranslation();
  try {
    const text = typeof data === "string" ? data : JSON.stringify(data, null, 2);
    if (format === "md") return <div className="norns-markdown"><CopyButton value={text} /><ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown></div>;
    const value = typeof data === "string" ? format === "json" ? JSON.parse(text) : parseYaml(text, { maxAliasCount: 100 }) : data;
    if (format === "json") return <JsonViewer value={value} />;
    const formatted = stringifyYaml(value, { indent: 2, lineWidth: 0 });
    return <div className="norns-code"><CopyButton value={formatted} /><pre>{formatted}</pre></div>;
  } catch {
    return <div><p className="notice warning">{t("norns.invalidContent")}</p><pre>{String(data)}</pre></div>;
  }
}

export function NornsPanel({ serverId, kind, connection, subjectId, switcher }: { serverId: string; kind: NornsViewKind; connection: ConnectionInput; subjectId: string; enabled?: boolean; switcher?: ReactNode }) {
  const { t, i18n } = useTranslation();
  const [format, setFormat] = useState<ViewFormat>("md");
  const isView = kind === "status" || kind === "trace";
  const entity = kind === "cognition" || kind === "exploration";
  const cacheKey = nornsViewCacheKey(serverId, subjectId, kind, isView ? format : "json");
  const cached = readNornsViewCache(cacheKey);
  const [parameter, setParameter] = useState(cached?.parameter ?? "");
  const [data, setData] = useState<unknown>(cached?.data ?? null);
  const [revision, setRevision] = useState(cached?.revision ?? 0);
  const [sourceFilename, setSourceFilename] = useState<string | null>(cached?.sourceFilename ?? null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const activeKey = useRef(cacheKey);
  const requestVersion = useRef(0);
  useEffect(() => {
    activeKey.current = cacheKey;
    const next = readNornsViewCache(cacheKey);
    if (next) setParameter(next.parameter);
    setData(next?.data ?? null); setRevision(next?.revision ?? 0); setSourceFilename(next?.sourceFilename ?? null);
    setError(""); setNotice(""); setLoading(false); setSaving(false);
    requestVersion.current++;
    return () => { activeKey.current = ""; requestVersion.current++; };
  }, [cacheKey]);
  const generate = async () => {
    if ((kind === "trace" || entity) && !parameter.trim() || loading) return;
    const requestKey = cacheKey, requestParameter = parameter, request = ++requestVersion.current;
    setLoading(true); setError(""); setNotice("");
    try {
      const result = kind === "status" ? await getNornsStatus(connection, subjectId, parameter.trim() || null, format)
        : kind === "trace" ? await getNornsTrace(connection, subjectId, parameter.trim(), format)
        : entity ? { data: await getNornsEntity(connection, subjectId, kind, parameter.trim()), filename: null }
        : await getNornsVisual(connection, subjectId, i18n.language.toLowerCase().startsWith("zh") ? "zh" : "en");
      if (activeKey.current !== requestKey || requestVersion.current !== request) return;
      const nextRevision = revision + 1;
      writeNornsViewCache(requestKey, { parameter: requestParameter, data: result.data, revision: nextRevision, sourceFilename: result.filename });
      setData(result.data); setRevision(nextRevision); setSourceFilename(result.filename);
    } catch (cause) {
      if (activeKey.current === requestKey && requestVersion.current === request) setError(errorMessage(cause, t));
    } finally { if (activeKey.current === requestKey && requestVersion.current === request) setLoading(false); }
  };
  const parameterLabel = t(kind === "status" ? "norns.taskId" : kind === "cognition" ? "norns.cognitionId" : "norns.explorationId");
  const saveView = async () => {
    if (data === null) return;
    const key = cacheKey;
    setSaving(true); setError(""); setNotice("");
    try { const path = await saveNornsView(kind, data, sourceFilename, entity ? "json" : format); if (path && activeKey.current === key) setNotice(t("norns.saved")); }
    catch (cause) { if (activeKey.current === key) setError(errorMessage(cause, t)); }
    finally { if (activeKey.current === key) setSaving(false); }
  };
  return <div className="norns-panel">
    <div className="panel-toolbar norns-controlbar">{switcher}<div className="norns-view-actions">
      {kind !== "visual" && <label className="norns-parameter"><Hash /><input aria-label={parameterLabel} placeholder={parameterLabel} value={parameter} onChange={(e) => { setParameter(e.target.value); writeNornsViewCache(cacheKey, { parameter: e.target.value, data, revision, sourceFilename }); }} spellCheck={false} /></label>}
      {isView && <select aria-label={t("norns.format")} value={format} onChange={(e) => setFormat(e.target.value as ViewFormat)}><option value="md">Markdown</option><option value="yaml">YAML</option><option value="json">JSON</option></select>}
      <button className="button primary norns-generate" disabled={loading || (kind === "trace" || entity) && !parameter.trim()} onClick={() => void generate()}>{data !== null ? <RefreshCw /> : <Play />}{entity ? t("norns.query") : data !== null ? t("norns.regenerate") : t("norns.generate")}</button>
      {data !== null && <button className="button secondary norns-save" disabled={loading || saving} onClick={() => void saveView()}><Save />{saving ? t("norns.saving") : t("norns.saveView")}</button>}
    </div></div>
    <div className="norns-view-content">{(error || notice) && <div className="norns-view-feedback">{error && <p className="notice warning" role="alert">{error}</p>}{notice && <p className="notice success">{notice}</p>}</div>}
      <div className="norns-view-result">{loading ? <div className="loading-state">{t("common.loading")}</div> : data !== null ? kind === "visual" && typeof data === "string" ? <iframe key={`${cacheKey}-${revision}`} className="visual-frame" sandbox="allow-scripts" srcDoc={data} title="Norns Visual" /> : <ViewPreview data={data} format={entity ? "json" : format} /> : <div className="empty-inline">{t("common.noData")}</div>}</div>
    </div>
  </div>;
}
