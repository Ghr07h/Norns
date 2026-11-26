import { Download } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listen } from "@tauri-apps/api/event";
import { cancelArchive, downloadArchive, listRuntimesPage } from "../../api/commands";
import type { ConnectionInput, RuntimeSummary, SubjectSummary } from "../../api/dto";
import { Dialog } from "../../components/common/Dialog";
import { errorMessage, formatBytes } from "../../lib/format";

type Kind = "root" | "subject" | "rundata" | "subject-rundata" | "runtime";
export function ArchiveDialog({ open, connection, subjects, selectedSubject, onClose }: { open: boolean; connection: ConnectionInput; subjects: SubjectSummary[]; selectedSubject: string | null; onClose: () => void }) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<Kind>("root");
  const [subjectId, setSubjectId] = useState(selectedSubject ?? "");
  const [runtimeId, setRuntimeId] = useState("");
  const [runtimes, setRuntimes] = useState<RuntimeSummary[]>([]);
  const [runtimeError, setRuntimeError] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");
  const [progress, setProgress] = useState<{ received: number; total: number | null } | null>(null);
  const requestId = useRef<string | null>(null);
  const [loadRevision, setLoadRevision] = useState(0);
  useEffect(() => { if (open && !busy) { setSubjectId(selectedSubject ?? ""); setError(""); setSaved(""); } }, [open, selectedSubject]);
  useEffect(() => {
    let mounted = true;
    let unlisten: (() => void) | undefined;
    void listen<{ request_id: string; received: number; total: number | null }>("well:archive-progress", (event) => {
      if (event.payload.request_id === requestId.current) setProgress(event.payload);
    }).then((stop) => { if (mounted) unlisten = stop; else stop(); }).catch(() => undefined);
    return () => { mounted = false; unlisten?.(); if (requestId.current) void cancelArchive(requestId.current); requestId.current = null; };
  }, []);
  useEffect(() => {
    let active = true;
    setRuntimes([]); setRuntimeId(""); setRuntimeError("");
    if (!open || kind !== "runtime" || !subjectId) return;
    void listRuntimesPage(connection, subjectId, null).then((page) => { if (active) setRuntimes(page.runtimes); }).catch((cause) => { if (active) setRuntimeError(errorMessage(cause, t)); });
    return () => { active = false; };
  }, [connection.base_url, connection.password, open, kind, subjectId, loadRevision]);
  const requiresSubject = ["subject", "subject-rundata", "runtime"].includes(kind);
  const start = async () => {
    if (busy) return;
    const id = crypto.randomUUID(); requestId.current = id;
    setBusy(true); setError(""); setSaved(""); setProgress(null);
    try {
      const path = await downloadArchive(connection, id, kind === "root" || kind === "subject" ? "subject" : "rundata", requiresSubject ? subjectId : undefined, kind === "runtime" ? runtimeId : undefined);
      if (requestId.current === id && path) setSaved(path);
    } catch (cause) {
      if (requestId.current === id) setError(errorMessage(cause, t));
    } finally { if (requestId.current === id) { requestId.current = null; setBusy(false); } }
  };
  const subject = subjects.find((s) => s.id === subjectId);
  return <Dialog open={open} title={t("archive.title")} onClose={busy ? () => undefined : onClose} footer={<>
    <button className="button secondary" onClick={() => { if (busy && requestId.current) void cancelArchive(requestId.current); else onClose(); }}>{busy ? t("archive.cancelDownload") : t("common.close")}</button>
    <button className="button primary" disabled={busy || requiresSubject && (!subjectId || subject?.state !== "idle") || kind === "runtime" && !runtimeId} onClick={() => void start()}><Download />{t("archive.download")}</button>
  </>}>
    <fieldset disabled={busy} className="form-grid form-fieldset"><label>{t("archive.scope")}<select value={kind} onChange={(e) => setKind(e.target.value as Kind)}>{(["root", "subject", "rundata", "subject-rundata", "runtime"] as Kind[]).map((value) => <option key={value} value={value}>{t(`archive.scopes.${value}`)}</option>)}</select></label>
      {requiresSubject && <label>{t("subject.id")}<select value={subjectId} onChange={(e) => setSubjectId(e.target.value)}><option value="">{t("archive.chooseSubject")}</option>{subjects.map((s) => <option key={s.id} value={s.id}>{s.name || s.id} · {t(`status.${s.state}`)}</option>)}</select></label>}
      {kind === "runtime" && <label>{t("subject.runtimeId")}<input list="archive-runtimes" value={runtimeId} onChange={(e) => setRuntimeId(e.target.value)} placeholder="Runtime UUID" /><datalist id="archive-runtimes">{runtimes.map((r) => <option key={r.id} value={r.id}>{r.state}</option>)}</datalist>{runtimeError && <><p className="form-error">{runtimeError}</p><button className="button secondary" onClick={() => setLoadRevision((v) => v + 1)}>{t("common.retry")}</button></>}</label>}
    </fieldset>
    <p>{t("archive.hint")}</p>
    {requiresSubject && subject && subject.state !== "idle" && <p className="notice warning">{t("errors.SUBJECT_NOT_IDLE")}</p>}
    {busy && <p role="status">{progress ? `${t("archive.downloading")} ${formatBytes(progress.received)}${progress.total ? ` / ${formatBytes(progress.total)}` : ""}` : t("archive.preparing")}</p>}
    {error && <p className="notice warning" role="alert">{error}</p>}
    {saved && <p className="notice success">{t("archive.saved")} <span className="breakable">{saved}</span></p>}
  </Dialog>;
}
