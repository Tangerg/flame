import i18next from "i18next";

// "zh" needs an explicit region so ICU picks the Simplified grammar variant.
export function bcp47(): string {
  const lng = i18next.language ?? "en";
  if (lng === "zh") return "zh-CN";
  if (lng === "zh-TW" || lng.toLowerCase() === "zh-tw") return "zh-TW";
  return lng;
}

// Intl formatters are expensive and asked for once per row on every render. Keyed on the
// LOCALE so switching language builds a new one rather than serving a stale one.
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

const relativeTimeCache = new Map<string, Intl.RelativeTimeFormat>();

function relative(value: number, unit: Intl.RelativeTimeFormatUnit): string {
  const locale = bcp47();
  const cached = relativeTimeCache.get(locale);
  if (cached) return cached.format(value, unit);
  const created = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  relativeTimeCache.set(locale, created);
  return created.format(value, unit);
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

/** Year only when it is not this one; 12- vs 24-hour comes from the app LOCALE, never the
 *  host OS. Returns "" on unparseable input. */
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

/** Clock time alone. Returns "" on unparseable input. */
export function formatClock(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";
  return dateTimeFormat("hm", { hour: "numeric", minute: "2-digit" }).format(d);
}

/** An identity for grouping, never shown. Formatted labels would tie grouping to the display
 *  locale; ISO strings group by UTC day — the wrong midnight for everyone west of it. */
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

  // `numeric: "auto"` emits "now" only for value 0, so the whole sub-minute window must
  // collapse to 0 — a 45s cliff would read 45-59s as "this minute".
  if (diffSec < 60) return relative(0, "second");
  if (diffMin < 60) return relative(-diffMin, "minute");
  if (diffHour < 24) return relative(-diffHour, "hour");
  if (diffDay < 7) return relative(-diffDay, "day");

  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return absolute(d, sameYear);
}
