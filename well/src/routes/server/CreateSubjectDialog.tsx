import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../components/common/Dialog";
import { createSubject } from "../../api/commands";
import type { ConnectionInput } from "../../api/dto";
import { errorMessage, formatBytes } from "../../lib/format";

export function CreateSubjectDialog({ open, connection, onClose, onCreated }: { open: boolean; connection: ConnectionInput; onClose: () => void; onCreated: (subjectId: string) => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [run, setRun] = useState(false);
  const [instruction, setInstruction] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const bytes = new TextEncoder().encode(instruction).byteLength;
  useEffect(() => { if (open) setError(""); }, [open]);
  const submit = async () => { setBusy(true); setError(""); try { const result = await createSubject(connection, name.trim() || null, run ? instruction : null); setName(""); setInstruction(""); setRun(false); onCreated(result.subject_id); onClose(); } catch (cause) { setError(`${errorMessage(cause, t)} ${t("create.uncertain")}`); } finally { setBusy(false); } };
  return <Dialog open={open} title={t("create.title")} onClose={busy ? () => undefined : onClose} footer={<><button className="button secondary" disabled={busy} onClick={onClose}>{t("common.cancel")}</button><button className="button primary" disabled={busy || run && (!instruction.trim() || bytes > 1_048_576)} onClick={() => void submit()}>{t("common.add")}</button></>}><fieldset className="form-grid form-fieldset" disabled={busy}><label><span>{t("create.name")}</span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} /></label><label className="check-row"><input type="checkbox" checked={run} onChange={(event) => setRun(event.target.checked)} /><span>{t("create.runNow")}</span></label>{run && <label><span>{t("create.instruction")}</span><textarea value={instruction} onChange={(event) => setInstruction(event.target.value)} /><small className={bytes > 1_048_576 ? "danger-text" : ""}>{formatBytes(bytes)} / 1 MiB</small></label>}{error && <p className="form-error">{error}</p>}</fieldset></Dialog>;
}
