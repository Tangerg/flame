import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import {
  bcp47,
  dayKey,
  formatClock,
  formatDateTime,
  formatDay,
  formatRelative,
} from "./relativeTime";

// Input is whatever the Runtime or a stored draft held. Each answers "" (or null) rather than
// throwing, because a row that throws takes the whole list with it.

const STRINGS = [
  "",
  " ",
  "not a date",
  "2026-13-45T99:99:99Z",
  "0000-00-00",
  "2026-07-07T10:00:00Z",
  "2026-07-07",
  "1970-01-01T00:00:00Z",
  "+275760-09-13T00:00:00Z",
  "-271821-04-20T00:00:00Z",
];

const NUMBERS = [
  0,
  -1,
  1,
  8.64e15,
  -8.64e15,
  8.64e15 + 1,
  Number.MAX_SAFE_INTEGER,
  NaN,
  Infinity,
  -Infinity,
];

const TEXT_FORMATTERS = [
  ["formatDateTime", formatDateTime],
  ["formatClock", formatClock],
  ["formatDay", formatDay],
  ["formatRelative", formatRelative],
] as const;

describe("timestamp formatting, over what the wire can carry", () => {
  it.each(TEXT_FORMATTERS.map(([name]) => name))("%s answers a string for anything", (name) => {
    const format = TEXT_FORMATTERS.find(([candidate]) => candidate === name)![1];
    const inputs: unknown[] = [
      ...STRINGS,
      ...NUMBERS,
      undefined,
      null,
      new Date(),
      new Date(NaN),
      new Date(8.64e15 + 1),
    ];
    for (const input of inputs) {
      expect(typeof format(input as Parameters<typeof format>[0])).toBe("string");
    }
    forEachSeed(300, (a) => {
      expect(typeof format(a.text() as Parameters<typeof format>[0])).toBe("string");
    });
  });

  it.each(TEXT_FORMATTERS.map(([name]) => name))('%s answers "" for no timestamp', (name) => {
    const format = TEXT_FORMATTERS.find(([candidate]) => candidate === name)![1];
    expect(format(undefined)).toBe("");
    expect(format(null)).toBe("");
    expect(format("")).toBe("");
    expect(format(NaN)).toBe("");
    expect(format(new Date(NaN))).toBe("");
  });

  it("answers something for a timestamp that does parse, so the above is not vacuous", () => {
    for (const [, format] of TEXT_FORMATTERS) {
      expect(format("2026-07-07T10:00:00Z").length).toBeGreaterThan(0);
    }
  });

  it("keys a day by LOCAL midnight, never UTC", () => {
    // Two instants in the same LOCAL day share a key; two in different local days never do.
    forEachSeed(400, (a) => {
      const base = new Date(2026, a.int(12), 1 + a.int(27), a.int(24), a.int(60));
      const sameDay = new Date(base);
      sameDay.setHours(a.int(24), a.int(60));
      expect(dayKey(base)).toBe(dayKey(sameDay));

      const nextDay = new Date(base);
      nextDay.setDate(nextDay.getDate() + 1);
      expect(dayKey(base)).not.toBe(dayKey(nextDay));
    });
  });

  it("has no day key for input it cannot read", () => {
    for (const input of [undefined, null, "", "not a date", NaN]) {
      expect(dayKey(input as Parameters<typeof dayKey>[0])).toBeNull();
    }
  });

  it("reads a timestamp in the future as the present, not as a negative age", () => {
    // Clock skew between hosts is ordinary; a row reading "in -3 days" is not.
    for (const ahead of [1_000, 60_000, 86_400_000, 30 * 86_400_000]) {
      const label = formatRelative(new Date(Date.now() + ahead));
      expect(label.length).toBeGreaterThan(0);
      expect(label).not.toMatch(/-\d/);
    }
  });

  it("moves through its buckets as an age grows, without repeating itself", () => {
    const now = Date.now();
    const at = (ms: number) => formatRelative(new Date(now - ms));
    const seconds = at(5_000);
    const minutes = at(5 * 60_000);
    const hours = at(5 * 3_600_000);
    const days = at(3 * 86_400_000);
    const absolute = at(30 * 86_400_000);
    for (const label of [seconds, minutes, hours, days, absolute]) {
      expect(label.length).toBeGreaterThan(0);
    }
    expect(new Set([seconds, minutes, hours, days, absolute]).size).toBe(5);
  });

  // Building a formatter per row is the cost this module exists to avoid, on a path that runs
  // for every session row, inbox item and notification, on every render.
  it("builds at most one Intl formatter per locale, however many rows ask", () => {
    const RelativeTimeFormat = Intl.RelativeTimeFormat;
    const DateTimeFormat = Intl.DateTimeFormat;
    let built = 0;
    class CountingRelative extends RelativeTimeFormat {
      constructor(...args: ConstructorParameters<typeof RelativeTimeFormat>) {
        built += 1;
        super(...args);
      }
    }
    class CountingDateTime extends DateTimeFormat {
      constructor(...args: ConstructorParameters<typeof DateTimeFormat>) {
        built += 1;
        super(...args);
      }
    }
    Object.defineProperty(Intl, "RelativeTimeFormat", {
      value: CountingRelative,
      configurable: true,
    });
    Object.defineProperty(Intl, "DateTimeFormat", { value: CountingDateTime, configurable: true });
    try {
      const now = Date.now();
      for (let row = 0; row < 200; row += 1) {
        formatRelative(new Date(now - row * 60_000));
        formatClock(new Date(now - row * 60_000));
      }
      // A cold cache builds a bounded handful of shapes. It must never grow with the rows.
      expect(built).toBeLessThanOrEqual(
        Intl.DateTimeFormat.supportedLocalesOf([bcp47()]).length + 4,
      );
    } finally {
      Object.defineProperty(Intl, "RelativeTimeFormat", {
        value: RelativeTimeFormat,
        configurable: true,
      });
      Object.defineProperty(Intl, "DateTimeFormat", { value: DateTimeFormat, configurable: true });
    }
  });
});
