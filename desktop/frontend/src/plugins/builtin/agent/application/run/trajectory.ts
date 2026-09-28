import { useEffect, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { createParameterizedDataQuery, type AgentItem, type AgentRunFact } from "@/plugins/sdk";
import type { AgentRunTreeNode } from "../view/runTree";
import { useActiveSessionRunTree, useActiveSessionTimeline } from "./runReadModel";

export interface TrajectoryQuery {
  sessionId: string;
  includeDescendants: boolean;
  cursor?: string;
  limit: number;
}

export interface ModelInvocation {
  callId: string;
  runId: string;
  segmentId: string;
  state: "started" | "completed" | "failed" | "unknown";
  startedAt: string;
  settledAt?: string;
  firstOutputLatencyMillis?: number;
  usage?: {
    inputTokens: number;
    outputTokens: number;
    cacheReadTokens?: number;
    cacheWriteTokens?: number;
    reasoningTokens?: number;
  };
}

export type TrajectoryEntry =
  | { type: "run"; occurredAt: string; run: AgentRunFact }
  | { type: "model"; occurredAt: string; model: ModelInvocation }
  | { type: "item"; occurredAt: string; item: AgentItem };

export interface TrajectoryPage {
  data: TrajectoryEntry[];
  nextCursor?: string;
}

export const TRAJECTORY_RUN_KEY = "trajectory-run";
export interface TrajectoryRunQuery {
  runId: string;
}
const useRun = createParameterizedDataQuery<TrajectoryRunQuery, AgentRunFact>(TRAJECTORY_RUN_KEY);
export function useTrajectoryRun(runId: string | undefined) {
  const params = useMemo(() => (runId ? { runId } : undefined), [runId]);
  return useRun(params);
}

export const TRAJECTORY_KEY = "session-trajectory";
const useTrajectoryPage = createParameterizedDataQuery<TrajectoryQuery, TrajectoryPage>(
  TRAJECTORY_KEY,
);

export function useSessionTrajectory(
  sessionId: string | null,
  includeDescendants: boolean,
  cursor?: string,
) {
  const client = useQueryClient();
  const timeline = useActiveSessionTimeline();
  const runs = useActiveSessionRunTree();
  const revision = JSON.stringify(runRevisions(runs));
  const params = useMemo(
    () => (sessionId ? { sessionId, includeDescendants, cursor, limit: 100 } : undefined),
    [sessionId, includeDescendants, cursor],
  );
  const query = useTrajectoryPage(params);
  const { refetch } = query;
  useEffect(() => {
    if (!params) return;
    let active = true;
    void client.cancelQueries({ queryKey: [TRAJECTORY_KEY, params], exact: true }).then(() => {
      if (active) void refetch();
    });
    return () => {
      active = false;
    };
  }, [client, params, refetch, revision, timeline]);
  return query;
}

function runRevisions(nodes: readonly AgentRunTreeNode[]): unknown[] {
  return nodes.map(({ run, children }) => [
    run.id,
    run.status,
    run.activeSegmentId,
    run.metrics.steps,
    run.progress?.step,
    run.progress?.activity,
    runRevisions(children),
  ]);
}
