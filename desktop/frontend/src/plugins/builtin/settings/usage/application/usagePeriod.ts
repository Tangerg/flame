const UsagePeriodKeyPart = {
  AllTime: "allTime",
  Recent: "recent",
} as const;

export type UsagePeriodKey =
  | readonly [typeof UsagePeriodKeyPart.AllTime]
  | readonly [typeof UsagePeriodKeyPart.Recent, number];

/**
 * UsagePeriod owns the distinction between all durable history and a recent,
 * positive calendar-day window. Callers cannot manufacture a zero-day period
 * and adapters never need to interpret numeric zero as absence.
 */
export class UsagePeriod {
  private constructor(private readonly days: number | undefined) {}

  static allTime(): UsagePeriod {
    return new UsagePeriod(undefined);
  }

  static recent(days: number): UsagePeriod {
    if (!Number.isSafeInteger(days) || days <= 0) {
      throw new Error("Recent usage period days must be a positive safe integer");
    }
    return new UsagePeriod(days);
  }

  recentDays(): number | undefined {
    return this.days;
  }

  cacheKey(): UsagePeriodKey {
    return this.days === undefined
      ? [UsagePeriodKeyPart.AllTime]
      : [UsagePeriodKeyPart.Recent, this.days];
  }
}

export const ALL_TIME_USAGE = UsagePeriod.allTime();

export function recentUsage(days: number): UsagePeriod {
  return UsagePeriod.recent(days);
}
