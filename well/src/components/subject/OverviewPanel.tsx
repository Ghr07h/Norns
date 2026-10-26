import { CheckCircle2, CircleDashed, Fingerprint, Folder, Gauge } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { SubjectDetail } from "../../api/dto";
import { CopyButton } from "../common/CopyButton";
import { StatusBadge } from "../common/StatusBadge";
import { formatTime } from "../../lib/format";

function ValueRow({ label, value, copy = false }: { label: string; value: string; copy?: boolean }) {
  return <div className="value-row"><dt>{label}</dt><dd className={copy ? "mono breakable" : ""}>{value}{copy && <CopyButton value={value} />}</dd></div>;
}

function TimeRow({ label, value, locale }: { label: string; value: string; locale: string }) {
  return <div className="value-row"><dt>{label}</dt><dd title={value}>{formatTime(value, locale)}<CopyButton value={value} /></dd></div>;
}

export function OverviewPanel({ subject }: { subject: SubjectDetail }) {
  const { t, i18n } = useTranslation();
  const path = "path" in subject.workspace ? subject.workspace.path : undefined;
  const nornsEnabled = subject.runtime_profile.capabilities.includes("norns");
  return <div className="panel-content overview-stack">
    <section className="overview-section"><div className="section-title"><Fingerprint /><h3>{t("subject.entityInfo")}</h3></div><dl><ValueRow label={t("subject.id")} value={subject.id} copy /><div className="value-row"><dt>{t("subject.profile")}</dt><dd className="profile-tags"><span className="profile-tag">{t("subject.runtimeAgent")} · {subject.runtime_profile.agent} {subject.runtime_profile.agent_version}</span><span className={`profile-tag ${nornsEnabled ? "enabled" : "disabled"}`}>{t("subject.nornsMode")} · {nornsEnabled ? t("subject.nornsEnabled") : t("subject.nornsDisabled")}</span></dd></div><TimeRow label={t("subject.created")} value={subject.created_at} locale={i18n.language} /><TimeRow label={t("subject.updated")} value={subject.updated_at} locale={i18n.language} /><ValueRow label={t("subject.lastEventSeq")} value={String(subject.last_event_seq)} copy /></dl></section>
    <section className="overview-section"><div className="section-title"><Folder /><h3>{t("subject.workspace")}</h3></div><dl><div className="value-row"><dt>{t("subject.materialized")}</dt><dd className="inline-value">{subject.workspace.materialized ? <CheckCircle2 /> : <CircleDashed />}{subject.workspace.materialized ? t("subject.materializedReady") : t("subject.materializedPending")}</dd></div><ValueRow label={t("subject.directory")} value={subject.workspace.directory_name} copy /><ValueRow label={t("subject.path")} value={path === undefined ? t("subject.notProvided") : path === null ? t("subject.notAvailable") : path} copy={typeof path === "string"} /></dl></section>
    <section className="overview-section"><div className="section-title"><Gauge /><h3>{t("subject.runtimeStatus")}</h3></div><dl><div className="value-row"><dt>{t("subject.runtimeOccupancy")}</dt><dd className="inline-value"><StatusBadge value={subject.state} /></dd></div></dl></section>
  </div>;
}
