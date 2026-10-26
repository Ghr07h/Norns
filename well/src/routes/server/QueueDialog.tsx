import { ListOrdered } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { QueueEntry } from "../../api/dto";
import { Dialog } from "../../components/common/Dialog";
import { formatTime, shortId } from "../../lib/format";

export function QueueDialog({ open, entries, loading, error, onClose, onSelect }: {
  open: boolean;
  entries: QueueEntry[];
  loading: boolean;
  error: unknown;
  onClose: () => void;
  onSelect: (subjectId: string) => void;
}) {
  const { t, i18n } = useTranslation();
  return <Dialog open={open} title={t("workspace.queue")} onClose={onClose} wide footer={<button className="button secondary" onClick={onClose}>{t("common.close")}</button>}>
    {error ? <p className="notice warning">{String(error)}</p> : loading ? <div className="loading-state">{t("common.loading")}</div> : entries.length ? <div className="queue-list">
      {entries.map((entry) => <button className="queue-row" key={entry.runtime_id} onClick={() => onSelect(entry.subject_id)}>
        <strong className="queue-position">#{entry.position}</strong>
        <span className="queue-subject"><strong>{entry.subject_name || t("subject.unnamed")}</strong><small className="mono">{entry.subject_id}</small></span>
        <span className="queue-runtime"><small>{t("subject.runtime")}</small><span className="mono">{shortId(entry.runtime_id)}</span></span>
        <span className="queue-instructions"><strong>{entry.instruction_count}</strong><small>{t("workspace.instructions")}</small></span>
        <time title={entry.queued_at}>{formatTime(entry.queued_at, i18n.language)}</time>
      </button>)}
    </div> : <div className="empty-inline"><ListOrdered />{t("workspace.queueEmpty")}</div>}
  </Dialog>;
}
