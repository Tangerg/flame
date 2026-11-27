import type { WorkflowAuthor } from "@/features/plugins/public/workflows";
import { type GoalTarget, type GoalStatus, type GoalReason } from "@flame/runtime-client/wire";
import { useAgentSessionSharedMaterial } from "@/features/agent/public/sessionMaterial";
import { type AgentProjectionMaterial } from "@/features/agent/public/sessionMaterial";
export type { GoalStatus };

interface GoalUsage {
  runs: number;
  costUsd?: number;
  steps: number;
}

export interface GoalReadModel extends GoalTarget {
  author?: WorkflowAuthor;
  objective: string;
  status: GoalStatus;
  reason?: GoalReason;
  used: GoalUsage;
  createdAt: string;
}

// The Session shared-material path every Goal projection is published under.
export const GOAL_MATERIAL = "goal";

export const GOAL_SLASH_COMMAND = "/goal";

export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

export function useGoalMaterial(): AgentProjectionMaterial<GoalState> {
  return useAgentSessionSharedMaterial<GoalState>(GOAL_MATERIAL);
}
