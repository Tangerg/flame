import { type GoalReadModel, type GoalStatus } from "@/features/goal/application/goalReadModel";
export const GOAL_STATUS_I18N = {
  active: { label: "goal.summary.active" },
  paused: { label: "goal.summary.paused" },
  blocked: { label: "goal.summary.blocked" },
  completing: { label: "goal.summary.completing" },
} as const satisfies Record<GoalStatus, { label: string }>;

export const GOAL_REASON_I18N = {
  stoppedByUser: { label: "goal.reason.stoppedByUser" },
  runtimeRestarted: { label: "goal.reason.runtimeRestarted" },
  runStartFailed: { label: "goal.reason.runStartFailed" },
  awaitingInput: { label: "goal.reason.awaitingInput" },
  terminalOutcomeMissing: { label: "goal.reason.terminalOutcomeMissing" },
  runNotCompleted: { label: "goal.reason.runNotCompleted" },
  blockedByModel: { label: "goal.reason.blockedByModel" },
} as const satisfies Record<NonNullable<GoalReadModel["reason"]>["code"], { label: string }>;

export function goalCanResume(goal: GoalReadModel): boolean {
  return goal.status === "paused" || goal.status === "blocked";
}
