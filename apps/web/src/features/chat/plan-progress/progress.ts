import { type PlanStep } from "@/features/agent/public/plan";
import { type SessionPlan } from "@/features/agent/public/plan";
export interface ActivePlanState {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanStep | undefined;
  // currentIndex is -1 when no step is in flight: a plan that is finished or not
  // started has a standing (the completion count), not a position to point at.
  currentIndex: number;
}

// The pill is the session's one plan surface, so it is bound to the plan's
// existence rather than to a running Run: a finished plan stays reviewable
// between turns, and an absent one shows nothing.
export function activePlanState(plan: SessionPlan): ActivePlanState {
  const { done, total } = plan.progress();
  const current = plan.activeStep();

  return {
    visible: total > 0,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current,
    currentIndex: current ? plan.steps.findIndex((step) => step.id === current.id) : -1,
  };
}
