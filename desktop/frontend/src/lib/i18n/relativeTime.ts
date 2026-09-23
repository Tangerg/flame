import i18next from "i18next";

export function bcp47(): string {
  const lng = i18next.language ?? "en";
  if (lng === "zh") return "zh-CN";
  if (lng === "zh-TW" || lng.toLowerCase() === "zh-tw") return "zh-TW";
  return lng;
}

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

export type ClockPrecision = "minute" | "second";

export function formatClock(
  input: string | number | Date | undefined | null,
  precision: ClockPrecision = "minute",
): string {
  const d = parse(input);
  if (!d) return "";
  return precision === "second"
    ? dateTimeFormat("hms", {
        hour: "numeric",
        minute: "2-digit",
        second: "2-digit",
      }).format(d)
    : dateTimeFormat("hm", { hour: "numeric", minute: "2-digit" }).format(d);
}

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

export function formatRelative(input: string | number | Date | undefined | null): string {
  const d = parse(input);
  if (!d) return "";

  const now = Date.now();
  const diffMs = now - d.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return relative(0, "second");
  if (diffMin < 60) return relative(-diffMin, "minute");
  if (diffHour < 24) return relative(-diffHour, "hour");
  if (diffDay < 7) return relative(-diffDay, "day");

  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return absolute(d, sameYear);
}
