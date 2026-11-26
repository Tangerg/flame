import { activeLocale, localeToBcp47, type Translate } from "@/lib/i18n/index";
const formatters = new Map<string, Intl.NumberFormat>();

function decimal(value: number, maximumFractionDigits: number, minimumFractionDigits = 0): string {
  const locale = localeToBcp47(activeLocale());
  const key = `${locale}/${minimumFractionDigits}/${maximumFractionDigits}`;
  let formatter = formatters.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, {
      useGrouping: false,
      minimumFractionDigits,
      maximumFractionDigits,
    });
    formatters.set(key, formatter);
  }
  return formatter.format(value);
}

export function fmtTokens(n: number): string {
  if (n < 1000) return decimal(n, 0);
  if (n < 1_000_000) return `${decimal(n / 1000, 1)}k`;
  return `${decimal(n / 1_000_000, 1, 1)}M`;
}

// The catalogs own each locale's units and spacing; whole seconds keep a ticking clock steady.
function clockText(t: Translate, seconds: number): string {
  if (seconds < 60) return t("duration.seconds", { sec: seconds });
  const min = Math.floor(seconds / 60);
  if (min < 60) return t("duration.minutes", { min, sec: seconds % 60 });
  const hr = Math.floor(min / 60);
  return t("duration.hours", { hr, min: min % 60 });
}

export function fmtDuration(t: Translate, ms: number): string {
  if (ms < 1000) return t("duration.milliseconds", { ms: Math.round(ms) });
  if (ms < 10_000) return t("duration.seconds", { sec: decimal(ms / 1000, 1) });
  return clockText(t, Math.round(ms / 1000));
}

export function durationText(t: Translate, start: number, end: number | null): string {
  if (end === null) return "—";
  return clockText(t, Math.round((end - start) / 1000));
}
