import { type AgentPlan, type AgentSessionView } from "@/features/agent/public/sessionView";
export function onPlanUpdated(state: AgentSessionView, plan: AgentPlan): AgentSessionView {
  if (state.plan && state.plan.revision >= plan.revision) return state;
  return { ...state, plan };
}
