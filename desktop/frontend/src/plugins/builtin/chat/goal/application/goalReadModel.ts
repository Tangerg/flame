import {
  useAgentSessionSharedMaterial,
  type AgentProjectionMaterial,
} from "@/plugins/builtin/agent/public/sessionMaterial";
import type { GoalBudget } from "./goalBudget";

export type GoalStatus = "active" | "paused" | "blocked" | "completing";

export interface GoalUsage {
  runs: number;
  costUsd: number;
  steps: number;
}

/**
 * Spelled in this context's own words rather than the wire enum: a read model publishing
 * the protocol's vocabulary makes every consumer of this key a consumer of the protocol.
 */
export type GoalStopCode =
  | "stoppedByUser"
  | "runtimeRestarted"
  | "runStartFailed"
  | "awaitingInput"
  | "terminalOutcomeMissing"
  | "runNotCompleted"
  | "runBudgetReached"
  | "costBudgetReached"
  | "stepBudgetReached"
  | "blockedByModel";

export interface GoalStop {
  code: GoalStopCode;
  detail: string;
}

export interface GoalReadModel {
  sessionId: string;
  objective: string;
  status: GoalStatus;
  /** Absent while the goal is still running. */
  stop: GoalStop | null;
  /** Null means the Goal has no budget boundary. */
  budget: GoalBudget | null;
  used: GoalUsage;
  provider: string;
  model: string;
  reasoningEffort: string;
  createdAt: string;
  updatedAt: string;
}

// The material folds three states into one shape: "feature off"
// (available=false, from capability discovery), "on, no goal", and "has a goal".
export interface GoalState {
  available: boolean;
  goal: GoalReadModel | null;
}

/** The active Session's Goal and the exact Agent projection generation that
 * admitted it. There is deliberately no independent Goal query or store. */
export function useGoalMaterial(): AgentProjectionMaterial<GoalState> {
  return useAgentSessionSharedMaterial<GoalState>("goal");
}
