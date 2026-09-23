import { activeLocale, type Translate } from "./i18n";

const formatters = new Map<string, Intl.NumberFormat>();

function decimal(value: number, fractionDigits: number, exact = false): string {
  const locale = activeLocale();
  const key = `${locale}/${fractionDigits}/${exact}`;
  let formatter = formatters.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, {
      useGrouping: false,
      minimumFractionDigits: exact ? fractionDigits : 0,
      maximumFractionDigits: fractionDigits,
    });
    formatters.set(key, formatter);
  }
  return formatter.format(value);
}

export function fmtTokens(n: number): string {
  if (n < 1000) return decimal(n, 0);
  if (n < 1_000_000) return `${decimal(n / 1000, 1)}k`;
  return `${decimal(n / 1_000_000, 1, true)}M`;
}

export function fmtCost(usd: number): string {
  if (usd > 0 && usd < 0.01) return `$${decimal(usd, 4, true)}`;
  return `$${decimal(usd, 2, true)}`;
}

export type DurationPrecision = "whole" | "tenths";

export function fmtDuration(ms: number, precision: DurationPrecision = "whole"): string {
  if (ms < 1000) return `${decimal(ms, precision === "tenths" ? 1 : 0)}ms`;
  const seconds = ms / 1000;
  if (seconds < 10) return `${decimal(Math.round(seconds * 10) / 10, 1)}s`;
  const whole = Math.round(seconds);
  if (whole < 60) return `${whole}s`;
  const minutes = Math.floor(whole / 60);
  if (minutes < 60) return `${minutes}m ${String(whole - minutes * 60).padStart(2, "0")}s`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${String(minutes - hours * 60).padStart(2, "0")}m`;
}

export function fmtMetric(value: number): string {
  return value < 10 ? decimal(value, 1) : decimal(Math.round(value), 0);
}

export function durationText(t: Translate, start: number, end: number | null): string {
  if (!end) return "—";
  const sec = Math.round((end - start) / 1000);
  if (sec < 60) return t("duration.seconds", { sec });
  const min = Math.floor(sec / 60);
  if (min < 60) return t("duration.minutes", { min, sec: sec % 60 });
  const hr = Math.floor(min / 60);
  return t("duration.hours", { hr, min: min % 60 });
}
