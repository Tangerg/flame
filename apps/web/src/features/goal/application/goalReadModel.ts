import { type GoalStatus, type GoalReason } from "@flame/runtime-client/wire";
import { useAgentSessionSharedMaterial } from "@/features/agent/public/sessionMaterial";
import { type AgentProjectionMaterial } from "@/features/agent/public/sessionMaterial";
export type { GoalStatus };

interface GoalUsage {
  runs: number;
  costUsd?: number;
  steps: number;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  reason?: GoalReason;
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
