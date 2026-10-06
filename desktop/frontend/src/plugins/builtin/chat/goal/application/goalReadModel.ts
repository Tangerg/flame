import {
  useAgentSessionSharedMaterial,
  type AgentProjectionMaterial,
} from "@/plugins/builtin/agent/public/sessionMaterial";

export type GoalStatus = "active" | "paused" | "blocked" | "completing";

interface GoalUsage {
  runs: number;
  costUsd?: number;
  steps: number;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  used: GoalUsage;
  createdAt: string;
}

export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

export function useGoalMaterial(): AgentProjectionMaterial<GoalState> {
  return useAgentSessionSharedMaterial<GoalState>("goal");
}
