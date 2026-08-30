import type { GoalReadModel, GoalStatus, GoalStopCode } from "./goalReadModel";

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

const EXHAUSTED_BUDGET_STOPS = new Set<GoalStopCode>([
  "runBudgetReached",
  "costBudgetReached",
  "stepBudgetReached",
]);

/** Runtime refuses resume once a durable budget cap is spent. All other
 * paused/blocked states retain the same Goal incarnation and are resumable. */
export function goalCanResume(goal: GoalReadModel): boolean {
  if (goal.status !== "paused" && goal.status !== "blocked") return false;
  return !goal.stop || !EXHAUSTED_BUDGET_STOPS.has(goal.stop.code);
}
