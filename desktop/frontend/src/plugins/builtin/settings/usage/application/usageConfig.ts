import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { usageGateway } from "./ports/usageGateway";
import { ALL_TIME_USAGE, recentUsage, type UsagePeriod } from "./usagePeriod";

export const USAGE_SUMMARY_KEY = "usage.summary";

export interface UsageBreakdownBucket {
  key: string;
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
  runs?: number;
}

export const UsageRange = {
  AllTime: "allTime",
  Last30Days: "last30Days",
  Last7Days: "last7Days",
} as const;

export type UsageRange = (typeof UsageRange)[keyof typeof UsageRange];

export const USAGE_RANGES: ReadonlyArray<Readonly<{ value: UsageRange; label: string }>> = [
  { value: UsageRange.AllTime, label: "usage.range.all" },
  { value: UsageRange.Last30Days, label: "usage.range.30d" },
  { value: UsageRange.Last7Days, label: "usage.range.7d" },
];

export function usagePeriodForRange(range: UsageRange): UsagePeriod {
  switch (range) {
    case UsageRange.AllTime:
      return ALL_TIME_USAGE;
    case UsageRange.Last30Days:
      return recentUsage(30);
    case UsageRange.Last7Days:
      return recentUsage(7);
  }
}

export function usageTokens(bucket: { inputTokens?: number; outputTokens?: number }): number {
  return (bucket.inputTokens ?? 0) + (bucket.outputTokens ?? 0);
}

export function useUsageReport(period: UsagePeriod) {
  return useQuery({
    queryKey: [USAGE_SUMMARY_KEY, ...period.cacheKey()],
    queryFn: ({ signal }) => usageGateway().loadSummary(period, signal),
    placeholderData: keepPreviousData,
  });
}
