import type { GoalReadModel, GoalStatus } from "./goalReadModel";

export const GOAL_STATUS_I18N = {
  active: { label: "goal.summary.active" },
  paused: { label: "goal.summary.paused" },
  blocked: { label: "goal.summary.blocked" },
  completing: { label: "goal.summary.completing" },
} as const satisfies Record<GoalStatus, { label: string }>;

export function goalCanResume(goal: GoalReadModel): boolean {
  return goal.status === "paused" || goal.status === "blocked";
}
