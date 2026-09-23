import type { PlanStep, SessionPlan } from "@/plugins/builtin/agent/public/plan";

export interface ActivePlanState {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanStep | undefined;
}

export function activePlanState(plan: SessionPlan, currentRunActive: boolean): ActivePlanState {
  const { done, total } = plan.progress();
  const current = plan.activeStep();

  return {
    visible: currentRunActive && current !== undefined,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current,
  };
}
