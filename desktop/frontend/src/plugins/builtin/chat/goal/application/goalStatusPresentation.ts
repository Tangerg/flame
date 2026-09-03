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
// different reason, which is why this is not named after the budget. Naming them here rather
// than in a bare set is what stops the row from removing the resume control in silence.
const REFUSED_BY_RUNTIME_STOPS: Partial<Record<GoalStopCode, string>> = {
  runBudgetReached: "goal.stopped.runBudget",
  costBudgetReached: "goal.stopped.costBudget",
  stepBudgetReached: "goal.stopped.stepBudget",
  pricingUnavailable: "goal.stopped.pricingUnavailable",
};

/** The i18n key naming why the Runtime will not resume this goal, or null when it will. */
export function goalRefusalLabel(goal: GoalReadModel): string | null {
  if (goal.status !== "paused" && goal.status !== "blocked") return null;
  return (goal.stop && REFUSED_BY_RUNTIME_STOPS[goal.stop.code]) ?? null;
}

/** All other paused/blocked states retain the same Goal incarnation and are resumable. */
export function goalCanResume(goal: GoalReadModel): boolean {
  if (goal.status !== "paused" && goal.status !== "blocked") return false;
  return goalRefusalLabel(goal) === null;
}
