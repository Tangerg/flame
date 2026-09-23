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

type GoalStopCode =
  | "stoppedByUser"
  | "runtimeRestarted"
  | "runStartFailed"
  | "awaitingInput"
  | "terminalOutcomeMissing"
  | "runNotCompleted"
  | "blockedByModel";

interface GoalStop {
  code: GoalStopCode;
  detail: string;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  stop: GoalStop | null;
  used: GoalUsage;
  provider: string;
  model: string;
  reasoningEffort: string;
  createdAt: string;
  updatedAt: string;
}

export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

export function useGoalMaterial(): AgentProjectionMaterial<GoalState> {
  return useAgentSessionSharedMaterial<GoalState>("goal");
}
