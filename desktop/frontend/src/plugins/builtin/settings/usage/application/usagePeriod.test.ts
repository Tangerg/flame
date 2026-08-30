import { describe, expect, it } from "vitest";
import { ALL_TIME_USAGE, UsagePeriod, recentUsage } from "./usagePeriod";

describe("UsagePeriod", () => {
  it("models all-time without a numeric sentinel", () => {
    expect(ALL_TIME_USAGE.recentDays()).toBeUndefined();
    expect(ALL_TIME_USAGE.cacheKey()).toEqual(["allTime"]);
  });

  it("owns a positive recent-day window", () => {
    const period = recentUsage(30);
    expect(period.recentDays()).toBe(30);
    expect(period.cacheKey()).toEqual(["recent", 30]);
  });

  it.each([0, -1, 1.5, Number.NaN, Number.POSITIVE_INFINITY, Number.MAX_SAFE_INTEGER + 1])(
    "rejects an invalid recent-day count %s",
    (days) => {
      expect(() => UsagePeriod.recent(days)).toThrow(
        "Recent usage period days must be a positive safe integer",
      );
    },
  );
});
