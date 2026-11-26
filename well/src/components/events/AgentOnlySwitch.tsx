import { useTranslation } from "react-i18next";

export function AgentOnlySwitch({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useTranslation();
  return <button type="button" role="switch" aria-checked={checked} className="agent-only-switch" title={t("events.agentOnlyDescription")} onClick={() => onChange(!checked)}>
    <span className="agent-only-track" aria-hidden="true"><span /></span>
    <span>{t("events.agentOnly")}</span>
  </button>;
}
