import { Eye, Globe2, Network } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ConnectionInput } from "../../api/dto";
import { NornsPanel } from "./NornsPanel";
import type { NornsViewKind } from "./view-cache";

export type { NornsViewKind } from "./view-cache";

export function NornsViewsPanel({ serverId, connection, subjectId, enabled, initialKind = "visual" }: { serverId: string; connection: ConnectionInput; subjectId: string; enabled: boolean; initialKind?: NornsViewKind }) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<NornsViewKind>(initialKind);
  useEffect(() => setKind(initialKind), [subjectId, initialKind]);
  const views = [
    ["visual", t("norns.viewVisual"), Eye],
    ["status", t("norns.viewStatus"), Globe2],
    ["trace", t("norns.viewTrace"), Network],
  ] as const;
  const switcher = <div className="segmented norns-view-switcher">{views.map(([value, label, Icon]) => <button type="button" key={value} className={kind === value ? "active" : ""} onClick={() => setKind(value)}><Icon />{label}</button>)}</div>;
  return <div className="norns-views-panel"><NornsPanel key={`${serverId}:${subjectId}:${kind}`} serverId={serverId} kind={kind} connection={connection} subjectId={subjectId} enabled={enabled} switcher={switcher} /></div>;
}
