// Native `Intl.RelativeTimeFormat` + `Intl.DateTimeFormat`, no library: both handle plurals
// and locale strings natively, which is the whole point.

import i18next from "i18next";

// i18next locale id → BCP-47. "zh" needs the explicit region so ICU picks the Simplified
// grammar variant; every other locale is already a BCP-47 primary subtag.
export function bcp47(): string {
  const lng = i18next.language ?? "en";
  if (lng === "zh") return "zh-CN";
  if (lng === "zh-TW" || lng.toLowerCase() === "zh-tw") return "zh-TW";
  return lng;
}

// Intl formatters are expensive to construct and are asked for once per message row. Keyed
// on the LOCALE so switching language rebuilds them instead of serving a stale one.
const dateTimeCache = new Map<string, Intl.DateTimeFormat>();

function dateTimeFormat(shape: string, opts: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const locale = bcp47();
  const key = `${locale}|${shape}`;
  const cached = dateTimeCache.get(key);
  if (cached) return cached;
  const created = new Intl.DateTimeFormat(locale, opts);
  dateTimeCache.set(key, created);
  return created;
}

function relative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  return new Intl.RelativeTimeFormat(bcp47(), { numeric: "auto" }).format(value, unit);
}

function absolute(d: Date, sameYear: boolean): string {
  return sameYear
    ? dateTimeFormat("md", { month: "short", day: "numeric" }).format(d)
    : dateTimeFormat("ymd", { year: "numeric", month: "short", day: "numeric" }).format(d);
}

function parse(input: string | number | Date | undefined | null): Date | null {
  if (input === undefined || input === null || input === "") return null;
  const d = input instanceof Date ? input : new Date(input);
  return Number.isNaN(d.getTime()) ? null : d;
}

/**
 * The year appears only when it is not this one, and the 12- vs 24-hour choice comes from the
 * app LOCALE, never a callsite or the host OS. Returns "" on unparseable input.
 */
export function formatDateTime(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  const sameYear = d.getFullYear() === new Date().getFullYear();
  return dateTimeFormat(sameYear ? "md-hm" : "ymd-hm", {
    ...(sameYear ? {} : { year: "numeric" }),
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(d);
}

/**
 * Clock time alone: the date is carried once by the day separator above, and repeating it
 * on every turn is noise.
 *
 * Returns "" on unparseable input so the caller can render a fallback.
 */
export function formatClock(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  return dateTimeFormat("hm", { hour: "numeric", minute: "2-digit" }).format(d);
}

/**
 * An identity for grouping, never shown: comparing formatted labels would tie the grouping
 * to the display locale, and comparing ISO strings groups by UTC day — the wrong midnight
 * for everyone west of it.
 */
export function dayKey(input: string | number | Date | undefined | null): string | null {
  const d = parse(input);
  if (!d) return null;
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

export function formatDay(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  const sameYear = d.getFullYear() === new Date().getFullYear();
  return absolute(d, sameYear);
}

/** Returns "" on unparseable input so the caller can render a fallback. */
export function formatRelative(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";

  const now = Date.now();
  const diffMs = now - d.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  // Intl's `numeric: "auto"` only emits "now" for value 0, so the WHOLE sub-minute window
  // collapses to 0 — a 45s cliff drops 45-59s into the minute branch as "this minute".
  if (diffSec < 60) return relative(0, "second");
  if (diffMin < 60) return relative(-diffMin, "minute");
  if (diffHour < 24) return relative(-diffHour, "hour");
  if (diffDay < 7) return relative(-diffDay, "day");

  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return absolute(d, sameYear);
}
