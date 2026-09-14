import { useEffect } from "react";
import { createParameterizedDataQuery } from "@/plugins/sdk";
import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";

export interface ModelInvocationQuery {
  runId: string;
  cursor?: string;
  limit: number;
}

export interface ModelInvocation {
  usage?: {
    inputTokens: number;
    outputTokens: number;
    cacheReadTokens: number;
    cacheWriteTokens: number;
    reasoningTokens: number;
  };
  callId: string;
  runId: string;
  segmentId: string;
  state: "started" | "completed" | "failed" | "unknown";
  startedAt: string;
  settledAt?: string;
}

export const MODEL_INVOCATIONS_KEY = "model-invocations";
const useInvocationPage = createParameterizedDataQuery<
  ModelInvocationQuery,
  { data: ModelInvocation[]; nextCursor?: string }
>(MODEL_INVOCATIONS_KEY);

export function useModelInvocations(run: AgentRunView, cursor?: string) {
  const query = useInvocationPage({ runId: run.id, cursor, limit: 50 });
  const { refetch } = query;
  // Committed Run progress invalidates the current page without retaining a
  // separate cache key for every model step or polling an idle Runtime.
  useEffect(() => {
    void refetch();
  }, [
    refetch,
    run.status,
    run.activeSegmentId,
    run.progress?.step,
    run.progress?.activity,
    run.metrics.steps,
  ]);
  return query;
}
