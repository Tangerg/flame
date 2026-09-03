import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { UsagePeriod } from "../usagePeriod";
interface UsageAmount {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

interface UsageBucket extends UsageAmount {
  key: string;
  runs?: number;
}

interface UsageSummaryReadModel {
  total: UsageAmount;
  byProvider?: UsageBucket[];
  byModel?: UsageBucket[];
  byDay?: UsageBucket[];
  sessions?: number;
  runs?: number;
}

export interface UsageGateway {
  loadSummary(period: UsagePeriod, signal?: AbortSignal): Promise<UsageSummaryReadModel>;
}

const port = createSingletonPort<UsageGateway>("Usage gateway is not configured");

export const configureUsageGateway = port.configure;
export const usageGateway = port.get;
