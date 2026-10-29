import { Circle, CircleAlert, CircleCheck, Clock3, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";

const normalStatuses = new Set(["online", "idle", "using", "running", "ended"]);
const pendingStatuses = new Set(["checking", "retrying", "queuing", "queued", "waiting"]);
const advisoryStatuses = new Set(["duplicate_identity"]);
const blockingStatuses = new Set(["offline", "unauthorized", "incompatible", "identity_changed", "error", "cancelled"]);

export function StatusBadge({ value }: { value: string }) {
  const { t } = useTranslation();
  const normal = normalStatuses.has(value);
  const pending = pendingStatuses.has(value);
  const advisory = advisoryStatuses.has(value);
  const blocking = blockingStatuses.has(value);
  const loading = value === "checking" || value === "retrying";
  const tone = normal ? "ok" : pending || advisory ? "warning" : blocking ? "danger" : "neutral";
  const Icon = loading ? LoaderCircle : advisory || blocking ? CircleAlert : pending ? Clock3 : normal ? CircleCheck : Circle;
  const label = t(`status.${value}`, { defaultValue: t(`servers.${value}`, { defaultValue: value }) });
  return <span className={`status-badge status-${tone}`}><Icon className={loading ? "spin" : ""} />{label}</span>;
}
