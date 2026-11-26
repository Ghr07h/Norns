import { useTranslation } from "react-i18next";
import type { RuntimeConfig } from "../../api/commands";

export function parseRuntimeConfig(text: string): { config?: RuntimeConfig; invalid: boolean } {
  if (!text.trim()) return { invalid: false };
  try {
    const value: unknown = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some((key) => !key.startsWith("AGENT_")) || Object.values(value).some((v) => v === null)) return { invalid: true };
    return { config: value as RuntimeConfig, invalid: false };
  } catch { return { invalid: true }; }
}

export function RuntimeConfigEditor({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return <details className="runtime-config-editor"><summary>{t("runtime.config")}</summary><p>{t("runtime.configHint")}</p><textarea aria-label={t("runtime.config")} value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} placeholder={'{\n  "AGENT_LLM_MODEL": "model-name"\n}'} />{parseRuntimeConfig(value).invalid && <p className="form-error" role="alert">{t("runtime.invalidConfig")}</p>}</details>;
}
