import type { AgentPlan, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";

export function onPlanUpdated(state: AgentSessionView, plan: AgentPlan): AgentSessionView {
  if (state.plan && state.plan.revision >= plan.revision) return state;
  return { ...state, plan };
}
