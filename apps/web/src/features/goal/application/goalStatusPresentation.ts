import { type GoalReason, type RunOutcomeType } from "@flame/runtime-client/wire";
import { type GoalReadModel, type GoalStatus } from "@/features/goal/application/goalReadModel";
export const GOAL_STATUS_I18N = {
  active: { label: "goal.summary.active" },
  paused: { label: "goal.summary.paused" },
  blocked: { label: "goal.summary.blocked" },
  completing: { label: "goal.summary.completing" },
} as const satisfies Record<GoalStatus, { label: string }>;

const GOAL_REASON_I18N = {
  stoppedByUser: "goal.reason.stoppedByUser",
  runtimeRestarted: "goal.reason.runtimeRestarted",
  runStartFailed: "goal.reason.runStartFailed",
  awaitingInput: "goal.reason.awaitingInput",
  terminalOutcomeMissing: "goal.reason.terminalOutcomeMissing",
  blockedByModel: "goal.reason.blockedByModel",
} as const satisfies Record<Exclude<GoalReason["code"], "runNotCompleted">, string>;

const GOAL_RUN_OUTCOME_I18N = {
  failed: "goal.reason.runNotCompleted.failed",
  canceled: "goal.reason.runNotCompleted.canceled",
  timedOut: "goal.reason.runNotCompleted.timedOut",
  lost: "goal.reason.runNotCompleted.lost",
} as const satisfies Record<Exclude<RunOutcomeType, "completed">, string>;

/** The wire admits a runNotCompleted reason only with an unsuccessful outcome. */
export function goalReasonLabel(reason: GoalReason): string {
  if (reason.code !== "runNotCompleted") return GOAL_REASON_I18N[reason.code];
  const outcome = reason.runOutcome;
  if (outcome === undefined || outcome === "completed") {
    throw new Error("runNotCompleted goal reason without an unsuccessful run outcome");
  }
  return GOAL_RUN_OUTCOME_I18N[outcome];
}

export function goalCanResume(goal: GoalReadModel): boolean {
  return goal.status === "paused" || goal.status === "blocked";
}

export function goalIsActive(goal: GoalReadModel): boolean {
  return goal.status === "active";
}

export function goalCanEdit(goal: GoalReadModel): boolean {
  return goal.status !== "completing";
}

export function goalCanChangeStatus(goal: GoalReadModel): boolean {
  return goalIsActive(goal) || goalCanResume(goal);
}

export function goalCreatedAtMillis(createdAt: string): number | null {
  const millis = Date.parse(createdAt);
  return Number.isNaN(millis) ? null : millis;
}
