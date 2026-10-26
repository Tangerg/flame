import { type AgentAccess } from "@/features/agent/public/agent";
import { type ApprovalRuleSummary } from "@/features/agent/public/approvalPolicy";
import { forgetRule, forgetRules } from "@/features/agent/public/approvalPolicy";
import { useApprovalMode, useApprovalRules } from "@/features/agent/public/approvalPolicy";
export function useApprovalModeConfig() {
  return useApprovalMode();
}

export function useApprovalRuleConfigs(sessionId: string | undefined) {
  return useApprovalRules({ sessionId });
}

export async function forgetApprovalRule(agentAccess: AgentAccess, id: string): Promise<void> {
  await forgetRule(agentAccess, id);
}

export async function forgetApprovalRules(
  agentAccess: AgentAccess,
  rules: ApprovalRuleSummary[],
): Promise<void> {
  return forgetRules(
    agentAccess,
    rules.map((rule) => rule.id),
  );
}
