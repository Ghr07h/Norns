export function formatTime(value: string | null | undefined, locale: string): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "medium" }).format(date);
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB"];
  let amount = value / 1024;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit += 1; }
  return `${amount.toFixed(amount >= 10 ? 1 : 2)} ${units[unit]}`;
}

export function shortId(value: string): string { return value.slice(0, 8); }

export function errorMessage(error: unknown, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (typeof error === "object" && error) {
    const candidate = error as { code?: string; message?: string; details?: { reason?: unknown } };
    if (candidate.code) {
      const base = t(`errors.${candidate.code}`, { defaultValue: candidate.message ?? candidate.code });
      return typeof candidate.details?.reason === "string" ? `${base} (${candidate.details.reason})` : base;
    }
    if (candidate.message) return t(`errors.${candidate.message}`, { defaultValue: candidate.message });
  }
  return error instanceof Error ? t(`errors.${error.message}`, { defaultValue: error.message }) : t("errors.generic");
}
