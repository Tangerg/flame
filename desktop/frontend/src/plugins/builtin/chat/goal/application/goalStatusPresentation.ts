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

// `Goal.Resume` rejects each of these outright. A cap that is spent stays spent, and a cost
// cap the Runtime cannot price is one it will not agree to enforce — the same refusal for a
// different reason, which is why this is not named after the budget.
const REFUSED_BY_RUNTIME_STOPS = new Set<GoalStopCode>([
  "runBudgetReached",
  "costBudgetReached",
  "stepBudgetReached",
  "pricingUnavailable",
]);

/** All other paused/blocked states retain the same Goal incarnation and are resumable. */
export function goalCanResume(goal: GoalReadModel): boolean {
  if (goal.status !== "paused" && goal.status !== "blocked") return false;
  return !goal.stop || !REFUSED_BY_RUNTIME_STOPS.has(goal.stop.code);
}
