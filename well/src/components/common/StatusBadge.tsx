import { Circle, CircleAlert, CircleCheck, Clock3, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";

export function StatusBadge({ value }: { value: string }) {
  const { t } = useTranslation();
  const active = ["online", "using", "running"].includes(value);
  const waiting = ["checking", "retrying", "queuing", "queued", "waiting"].includes(value);
  const bad = ["offline", "unauthorized", "incompatible", "identity_changed", "duplicate_identity", "error", "cancelled"].includes(value);
  const loading = value === "checking" || value === "retrying";
  const Icon = loading ? LoaderCircle : active ? CircleCheck : waiting ? Clock3 : bad ? CircleAlert : Circle;
  const label = t(`status.${value}`, { defaultValue: t(`servers.${value}`, { defaultValue: value }) });
  return <span className={`status-badge status-${active ? "ok" : waiting ? "wait" : bad ? "bad" : "neutral"}`}><Icon className={loading ? "spin" : ""} />{label}</span>;
}
