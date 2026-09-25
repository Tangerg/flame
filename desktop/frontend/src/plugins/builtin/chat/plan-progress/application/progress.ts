import type { PlanStep, SessionPlan } from "@/plugins/builtin/agent/public/plan";

export interface ActivePlanState {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanStep | undefined;
}

// The pill is the session's one plan surface, so it is bound to the plan's
// existence rather than to a running Run: a finished plan stays reviewable
// between turns, and an absent one shows nothing.
export function activePlanState(plan: SessionPlan): ActivePlanState {
  const { done, total } = plan.progress();

  return {
    visible: total > 0,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current: plan.activeStep(),
  };
}
