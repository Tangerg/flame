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

type TimeUnit = "millisecond" | "second" | "minute" | "hour";
const timeUnits = new Map<string, Intl.NumberFormat>();
const timeLists = new Map<string, Intl.ListFormat>();

function timeUnit(value: number, unit: TimeUnit, maximumFractionDigits = 0): string {
  const locale = localeToBcp47(activeLocale());
  const key = `${locale}/${unit}/${maximumFractionDigits}`;
  let formatter = timeUnits.get(key);
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, {
      style: "unit",
      unit,
      unitDisplay: "narrow",
      useGrouping: false,
      maximumFractionDigits,
    });
    timeUnits.set(key, formatter);
  }
  return formatter.format(value);
}

function timeParts(...parts: string[]): string {
  const locale = localeToBcp47(activeLocale());
  let formatter = timeLists.get(locale);
  if (!formatter) {
    formatter = new Intl.ListFormat(locale, { type: "unit", style: "narrow" });
    timeLists.set(locale, formatter);
  }
  return formatter.format(parts);
}

export function fmtDuration(ms: number): string {
  if (ms < 1000) return timeUnit(ms, "millisecond");
  const seconds = ms / 1000;
  if (seconds < 10) return timeUnit(Math.round(seconds * 10) / 10, "second", 1);
  const whole = Math.round(seconds);
  if (whole < 60) return timeUnit(whole, "second");
  const minutes = Math.floor(whole / 60);
  if (minutes < 60) return timeParts(timeUnit(minutes, "minute"), timeUnit(whole % 60, "second"));
  const hours = Math.floor(minutes / 60);
  return timeParts(timeUnit(hours, "hour"), timeUnit(minutes % 60, "minute"));
}

export function durationText(t: Translate, start: number, end: number | null): string {
  if (end === null) return "—";
  const sec = Math.round((end - start) / 1000);
  if (sec < 60) return t("duration.seconds", { sec });
  const min = Math.floor(sec / 60);
  if (min < 60) return t("duration.minutes", { min, sec: sec % 60 });
  const hr = Math.floor(min / 60);
  return t("duration.hours", { hr, min: min % 60 });
}
