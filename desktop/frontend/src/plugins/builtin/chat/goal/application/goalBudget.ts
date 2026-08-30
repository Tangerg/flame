/** A finite Goal budget. At least one positive limit is present; omitting the
 * budget altogether represents an unlimited Goal. Runtime wire validation owns
 * positivity, while this union prevents application code from constructing an
 * empty finite budget. */
export type GoalBudget =
  | { maxRuns: number; maxCostUsd?: number; maxSteps?: number }
  | { maxRuns?: number; maxCostUsd: number; maxSteps?: number }
  | { maxRuns?: number; maxCostUsd?: number; maxSteps: number };
