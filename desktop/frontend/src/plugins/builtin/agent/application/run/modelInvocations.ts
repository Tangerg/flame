import { useEffect, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
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
  const client = useQueryClient();
  const params = useMemo(() => ({ runId: run.id, cursor, limit: 50 }), [run.id, cursor]);
  const query = useInvocationPage(params);
  const { refetch } = query;
  // Committed Run progress invalidates the current page without retaining a
  // separate cache key for every model step or polling an idle Runtime.
  useEffect(() => {
    let active = true;
    // Refetch alone joins an unfinished first read, which may predate this
    // progress. Retire that reader before requesting the committed state.
    void client
      .cancelQueries({ queryKey: [MODEL_INVOCATIONS_KEY, params], exact: true })
      .then(() => {
        if (active) void refetch();
      });
    return () => {
      active = false;
    };
  }, [
    client,
    params,
    refetch,
    run.status,
    run.activeSegmentId,
    run.progress?.step,
    run.progress?.activity,
    run.metrics.steps,
  ]);
  return query;
}
