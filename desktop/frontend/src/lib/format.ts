import { activeLocale } from "./i18n";

// The NUMBER follows the locale; the UNIT is notation. `toFixed` always writes a period,
// wrong in five of the eight locales here. NOT `Intl` compact notation: Japanese counts in
// 万, changing magnitude word and width per language inside a mono column.
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

// Sub-cent spend keeps 4 dp: rounding to "$0.00" would imply free.
export function fmtCost(usd: number): string {
  if (usd > 0 && usd < 0.01) return `$${decimal(usd, 4, true)}`;
  return `$${decimal(usd, 2, true)}`;
}

export function fmtDuration(ms: number): string {
  const seconds = ms / 1000;
  if (seconds < 10) return `${decimal(Math.round(seconds * 10) / 10, 1)}s`;
  // Rounded BEFORE the minute test: 59.6s rounds to 60, and no clock reads "60s".
  const whole = Math.round(seconds);
  if (whole < 60) return `${whole}s`;
  const minutes = Math.floor(whole / 60);
  if (minutes < 60) return `${minutes}m ${String(whole - minutes * 60).padStart(2, "0")}s`;
  // A clock drops its finest unit as its coarsest grows: a tool call that ran for six and a
  // half hours reads 6h 30m, not 390m 00s. Agent work is expected to run this long.
  const hours = Math.floor(minutes / 60);
  return `${hours}h ${String(minutes - hours * 60).padStart(2, "0")}m`;
}
