import type { ApprovalRuleSummary } from "@/plugins/builtin/agent/public/approvalPolicy";
import {
  APPROVAL_MODES,
  forgetRule,
  forgetRules,
  type ApprovalMode,
  useApprovalMode,
  useApprovalRules,
} from "@/plugins/builtin/agent/public/approvalPolicy";

export type { ApprovalMode };
export { APPROVAL_MODES };

export function useApprovalModeConfig() {
  return useApprovalMode();
}

export function useApprovalRuleConfigs(sessionId: string | undefined) {
  return useApprovalRules({ sessionId });
}

export async function forgetApprovalRule(id: string): Promise<void> {
  await forgetRule(id);
}

export async function forgetApprovalRules(rules: ApprovalRuleSummary[]): Promise<void> {
  return forgetRules(rules.map((rule) => rule.id));
}
