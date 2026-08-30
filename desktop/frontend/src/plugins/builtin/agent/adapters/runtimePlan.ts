import type { Plan as RuntimePlan, PlanStep as RuntimePlanStep } from "@/rpc";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";

const PLAN_STATUS: Record<RuntimePlanStep["status"], PlanStep["status"]> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

export function runtimePlan(plan: RuntimePlan): AgentPlan | undefined {
  if (!plan.state) return undefined;
  return {
    revision: plan.state.revision,
    steps: plan.state.steps.map((step) => ({
      id: step.id,
      text: step.description,
      status: PLAN_STATUS[step.status],
    })),
  };
}

export function runtimePlanUpdate(plan: RuntimePlan): AgentPlan {
  const projected = runtimePlan(plan);
  if (!projected) throw new Error("plan.updated has no committed state");
  return projected;
}
