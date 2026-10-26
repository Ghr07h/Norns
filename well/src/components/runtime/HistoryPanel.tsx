import { ChevronDown, ChevronRight, FileText } from "lucide-react";
import { Fragment, useEffect, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { ConnectionInput } from "../../api/dto";
import { getRuntime, listRuntimesPage } from "../../api/commands";
import { keys } from "../../api/query-keys";
import { CopyButton } from "../common/CopyButton";
import { StatusBadge } from "../common/StatusBadge";
import { formatTime } from "../../lib/format";

function ArtifactPath({ label, value, fallback }: { label: string; value: string | null | undefined; fallback: string }) {
  return <div className="artifact-row"><dt>{label}</dt><dd className="mono breakable">{value ?? fallback}{typeof value === "string" && <CopyButton value={value} />}</dd></div>;
}

function endReasonKey(value: string): string {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
}

export function HistoryPanel({ serverId, subjectId, connection }: { serverId: string; subjectId: string; connection: ConnectionInput }) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const history = useInfiniteQuery({ queryKey: keys.runtimes(serverId, subjectId), queryFn: ({ pageParam }) => listRuntimesPage(connection, subjectId, pageParam), initialPageParam: null as string | null, getNextPageParam: (page) => page.next_cursor });
  const detail = useQuery({ queryKey: keys.runtime(serverId, subjectId, selected ?? ""), queryFn: () => getRuntime(connection, subjectId, selected!), enabled: Boolean(selected) });
  const items = history.data?.pages.flatMap((page) => page.runtimes) ?? [];
  const reportedTotal = history.data?.pages[0]?.total_count;
  const totalCount = typeof reportedTotal === "number" && reportedTotal >= items.length ? reportedTotal : items.length;
  useEffect(() => { setOpen(false); setSelected(null); }, [subjectId]);
  const artifacts = detail.data?.artifacts ?? undefined;
  const transcriptFallback = artifacts?.transcript_status === "missing" ? t("runtime.transcriptMissing")
    : artifacts?.transcript_status === "failed" ? t("runtime.transcriptFailed")
    : t("runtime.transcriptPending");
  const reason = detail.data?.end_reason;

  return <section className="runtime-history">
    <button type="button" className="runtime-history-toggle" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
      {open ? <ChevronDown /> : <ChevronRight />}<strong>{t("runtime.records")}</strong><span>{t("runtime.recordsCount", { count: totalCount })}</span>
    </button>
    {open && <div className="runtime-records">
      {history.isPending ? <div className="loading-state compact">{t("common.loading")}</div> : items.length === 0 ? <div className="empty-inline compact">{t("common.noData")}</div> : items.map((runtime) => {
        const expanded = selected === runtime.id;
        return <Fragment key={runtime.id}>
          <button type="button" className={`runtime-record ${expanded ? "expanded" : ""}`} aria-expanded={expanded} onClick={() => setSelected(expanded ? null : runtime.id)}>
            {expanded ? <ChevronDown /> : <ChevronRight />}
            <span className="runtime-record-id mono" title={runtime.id}>{runtime.id}</span>
            <span className="runtime-record-time"><small>{t("runtime.startedAt")}</small>{formatTime(runtime.started_at ?? runtime.created_at, i18n.language)}</span>
            <span className="runtime-record-time"><small>{t("runtime.endedAt")}</small>{formatTime(runtime.ended_at, i18n.language)}</span>
            <StatusBadge value={runtime.state} />
          </button>
          {expanded && <div className="runtime-record-detail">
            {detail.isPending ? <div className="loading-state compact">{t("common.loading")}</div> : detail.data ? <>
              <div className="detail-section-header"><div><FileText /><h3>{t("runtime.details")}</h3></div></div>
              <dl className="detail-values">
                <div className="value-row"><dt>{t("runtime.endReason")}</dt><dd>{reason ? t(`runtime.endReasons.${endReasonKey(reason)}`, { defaultValue: reason }) : "-"}</dd></div>
                <div className="value-row"><dt>{t("subject.runtimeId")}</dt><dd className="mono breakable">{detail.data.id}<CopyButton value={detail.data.id} /></dd></div>
                <div className="value-row"><dt>{t("subject.sessionId")}</dt><dd className="mono breakable">{detail.data.session_id}<CopyButton value={detail.data.session_id} /></dd></div>
                <div className="value-row"><dt>{t("runtime.queuedAt")}</dt><dd title={detail.data.queued_at ?? ""}>{formatTime(detail.data.queued_at, i18n.language)}{detail.data.queued_at && <CopyButton value={detail.data.queued_at} />}</dd></div>
                <div className="value-row"><dt>{t("runtime.startedAt")}</dt><dd title={detail.data.started_at ?? ""}>{formatTime(detail.data.started_at, i18n.language)}{detail.data.started_at && <CopyButton value={detail.data.started_at} />}</dd></div>
                <div className="value-row"><dt>{t("runtime.endedAt")}</dt><dd title={detail.data.ended_at ?? ""}>{formatTime(detail.data.ended_at, i18n.language)}{detail.data.ended_at && <CopyButton value={detail.data.ended_at} />}</dd></div>
              </dl>
              <section className="artifact-section"><h4>{t("runtime.artifacts")}</h4><dl>
                <ArtifactPath label={t("runtime.rawLog")} value={artifacts?.raw_log_path} fallback={t("subject.notProvided")} />
                <ArtifactPath label={t("runtime.readableLog")} value={artifacts?.readable_log_path} fallback={t("subject.notProvided")} />
                <ArtifactPath label={t("runtime.transcript")} value={artifacts?.transcript_path} fallback={transcriptFallback} />
              </dl></section>
            </> : <div className="notice danger">{String(detail.error)}</div>}
          </div>}
        </Fragment>;
      })}
      {history.hasNextPage && <button className="button secondary load-more" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>{t("runtime.loadMore")}</button>}
    </div>}
  </section>;
}
