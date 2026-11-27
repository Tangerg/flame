import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type ApprovalRuleSummary } from "@/features/agent/application/approvalPolicyQueries";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/features/agent/application/approvalPolicyQueries";
import { type ApprovalMode, type ApprovalModeResult } from "@/features/agent/domain/hitl";
import { replaceCachedRead } from "@/lib/queryClient";

import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
type ApprovalActions = Pick<AgentDependencies, "commands" | "runtime">;

export function setApprovalMode(
  agentAccess: ApprovalActions,
  mode: ApprovalMode,
): Promise<ApprovalModeResult> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  return owner.serializeApprovalMode(async () => {
    try {
      return await owner.settle(runtime.setApprovalMode(mode));
    } finally {
      refreshPolicy(owner, APPROVAL_MODE_KEY);
    }
  });
}

export function forgetRules(
  agentAccess: ApprovalActions,
  rules: readonly Pick<ApprovalRuleSummary, "id" | "decision">[],
): Promise<void> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  return owner.serializeApprovalRules(async () => {
    try {
      for (const { id, decision } of rules) {
        await owner.settle(runtime.forgetApprovalRule(id, decision));
        owner.assertCurrent();
      }
    } finally {
      refreshPolicy(owner, APPROVAL_RULES_KEY);
    }
  });
}

function refreshPolicy(owner: AgentCommandOwner, queryKey: string): void {
  if (owner.isCurrent()) void replaceCachedRead({ queryKey: [queryKey] });
}
