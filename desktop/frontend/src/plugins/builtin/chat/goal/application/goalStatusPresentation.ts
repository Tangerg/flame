import type { GoalReadModel, GoalStatus } from "./goalReadModel";

/**
 * A paused goal is a thing to notice, a blocked one a thing to fix. Exhaustive so a new
 * Runtime lifecycle state cannot fall through to an untranslated key.
 */
export const GOAL_STATUS_I18N = {
  active: { label: "goal.summary.active" },
  paused: { label: "goal.summary.paused" },
  blocked: { label: "goal.summary.blocked" },
  completing: { label: "goal.summary.completing" },
} as const satisfies Record<GoalStatus, { label: string }>;

export function goalCanResume(goal: GoalReadModel): boolean {
  return goal.status === "paused" || goal.status === "blocked";
}
