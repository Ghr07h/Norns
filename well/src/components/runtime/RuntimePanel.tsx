import { CircleStop, OctagonX, SendHorizontal } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ConnectionInput, Runtime, SubjectDetail } from "../../api/dto";
import { stopRuntime, submitInstruction } from "../../api/commands";
import { keys } from "../../api/query-keys";
import { StatusBadge } from "../common/StatusBadge";
import { CopyButton } from "../common/CopyButton";
import { Dialog } from "../common/Dialog";
import { HistoryPanel } from "./HistoryPanel";
import { errorMessage, formatBytes, formatTime } from "../../lib/format";

export function RuntimePanel({ serverId, connection, subject, runtime }: { serverId: string; connection: ConnectionInput; subject: SubjectDetail; runtime: Runtime | null }) {
  const { t, i18n } = useTranslation();
  const client = useQueryClient();
  const [instruction, setInstruction] = useState("");
  const [instructionOpen, setInstructionOpen] = useState(false);
  const [stopMode, setStopMode] = useState<"graceful" | "force" | null>(null);
  const [notice, setNotice] = useState("");
  const bytes = new TextEncoder().encode(instruction).byteLength;
  const active = Boolean(runtime && ["queued", "running", "waiting"].includes(runtime.state));
  const closing = Boolean(runtime?.closing || runtime && ["ended", "error", "cancelled"].includes(runtime.state));
  const refresh = async () => { await Promise.all([client.invalidateQueries({ queryKey: keys.subjects(serverId) }), client.invalidateQueries({ queryKey: keys.subject(serverId, subject.id) }), client.invalidateQueries({ queryKey: keys.currentRuntime(serverId, subject.id) }), client.invalidateQueries({ queryKey: keys.runtimes(serverId, subject.id) }), client.invalidateQueries({ queryKey: keys.queue(serverId) }), client.invalidateQueries({ queryKey: keys.probe(serverId) })]); };
  const send = useMutation({ mutationFn: () => submitInstruction(connection, subject.id, instruction), onSuccess: async () => { setInstruction(""); setInstructionOpen(false); setNotice(""); await refresh(); }, onError: (error) => { const uncertain = typeof error === "object" && error && "kind" in error && ["timeout", "connection"].includes(String((error as { kind?: string }).kind)); setNotice(`${errorMessage(error, t)}${uncertain ? ` ${t("runtime.uncertain")}` : ""}`); } });
  const stop = useMutation({ mutationFn: (mode: "graceful" | "force") => stopRuntime(connection, subject.id, mode), onSuccess: async () => { setStopMode(null); setNotice(t("runtime.accepted")); await refresh(); }, onError: (error) => setNotice(errorMessage(error, t)) });
  const occupancy = runtime ? "using" : "idle";
  const currentState = runtime?.state === "queued" ? "queuing" : "using";
  const queuedAt = runtime?.queued_at ?? runtime?.created_at;

  return <div className="panel-content runtime-panel">
    <div className="runtime-commandbar">
      <StatusBadge value={occupancy} />
      <div className="runtime-actions">
        <button className="button primary" disabled={closing} onClick={() => setInstructionOpen(true)}><SendHorizontal />{t("runtime.sendInstruction")}</button>
        {active && <button className="button secondary" disabled={stop.isPending || runtime?.closing} onClick={() => setStopMode("graceful")}><CircleStop />{t("runtime.graceful")}</button>}
        {active && <button className="button danger" disabled={stop.isPending || runtime?.closing} onClick={() => setStopMode("force")}><OctagonX />{t("runtime.force")}</button>}
      </div>
    </div>
    {notice && <p className="notice">{notice}</p>}
    <HistoryPanel serverId={serverId} subjectId={subject.id} connection={connection} />
    {runtime && <section className="current-runtime">
      <div className="section-title"><h3>{t("runtime.currentDetails")}</h3></div>
      <dl>
        <div className="value-row"><dt>{t("subject.runtimeId")}</dt><dd className="mono breakable">{runtime.id}<CopyButton value={runtime.id} /></dd></div>
        <div className="value-row"><dt>{t("subject.runtimeStatus")}</dt><dd className="inline-value"><StatusBadge value={currentState} /></dd></div>
        <div className="value-row"><dt>{t("runtime.queuedAt")}</dt><dd title={queuedAt}>{formatTime(queuedAt, i18n.language)}{queuedAt && <CopyButton value={queuedAt} />}</dd></div>
        <div className="value-row"><dt>{t("runtime.startedAt")}</dt><dd title={runtime.started_at ?? ""}>{formatTime(runtime.started_at, i18n.language)}{runtime.started_at && <CopyButton value={runtime.started_at} />}</dd></div>
        <div className="value-row"><dt>{t("runtime.endedAt")}</dt><dd title={runtime.ended_at ?? ""}>{formatTime(runtime.ended_at, i18n.language)}{runtime.ended_at && <CopyButton value={runtime.ended_at} />}</dd></div>
        <div className="value-row"><dt>{t("runtime.closingLabel")}</dt><dd>{runtime.closing ? t("common.yes") : t("common.no")}</dd></div>
      </dl>
      <div className="pending-instructions"><h4>{t("runtime.pendingInstructions")}</h4>{runtime.instructions.length ? <ol>{runtime.instructions.map((item) => <li key={item.sequence}><div>{item.content}</div><time title={item.created_at}>{formatTime(item.created_at, i18n.language)}</time></li>)}</ol> : <p>{t("runtime.noPendingInstructions")}</p>}</div>
    </section>}
    <Dialog open={instructionOpen} title={t("runtime.sendInstruction")} description={runtime ? t("runtime.appendHint") : t("runtime.startHint")} onClose={() => setInstructionOpen(false)} footer={<><button className="button secondary" onClick={() => setInstructionOpen(false)}>{t("common.cancel")}</button><button className="button primary" disabled={send.isPending || !instruction.trim() || bytes > 1_048_576} onClick={() => send.mutate()}><SendHorizontal />{t("runtime.send")}</button></>}>
      <label className="dialog-field"><span>{t("runtime.instruction")}</span><textarea autoFocus value={instruction} onChange={(event) => setInstruction(event.target.value)} placeholder={t("runtime.instructionHint")} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && instruction.trim() && bytes <= 1_048_576) send.mutate(); }} /></label>
      <div className={`instruction-size ${bytes > 1_048_576 ? "danger-text" : ""}`}>{formatBytes(bytes)} / 1 MiB</div>
    </Dialog>
    <Dialog open={stopMode !== null} title={stopMode === "force" ? t("runtime.forceTitle") : t("runtime.gracefulTitle")} onClose={() => setStopMode(null)} footer={<><button className="button secondary" onClick={() => setStopMode(null)}>{t("common.cancel")}</button><button className={`button ${stopMode === "force" ? "danger" : "primary"}`} disabled={stop.isPending || stopMode === null} onClick={() => { if (stopMode) stop.mutate(stopMode); }}>{stopMode === "force" ? t("runtime.force") : t("runtime.graceful")}</button></>}>
      <p>{stopMode === "force" ? t("runtime.forceBody") : t("runtime.gracefulBody")}</p>
    </Dialog>
  </div>;
}
