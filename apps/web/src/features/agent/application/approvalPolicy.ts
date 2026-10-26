import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type MCPServerID } from "@/features/mcp/public/serverIdentity";
import { type ApprovalRuleSummary } from "@/features/agent/application/approvalPolicyQueries";
import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
} from "@/features/agent/application/approvalPolicyQueries";
import { type ApprovalMode, type ApprovalModeResult } from "@/features/agent/domain/hitl";
import { queryClient } from "@/lib/queryClient";

import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export function setApprovalMode(
  agentAccess: AgentDependencies,
  mode: ApprovalMode,
): Promise<ApprovalModeResult> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  return owner.serializeApprovalMode(async () => {
    let saved: ApprovalModeResult;
    try {
      saved = await owner.settle(runtime.setApprovalMode(mode));
    } catch (error) {
      if (!owner.isCurrent()) throw error;
      await repairProjection(owner, APPROVAL_MODE_KEY);
      throw error;
    }
    owner.assertCurrent();
    queryClient.setQueryData([APPROVAL_MODE_KEY], saved);
    await repairProjection(owner, APPROVAL_MODE_KEY);
    owner.assertCurrent();
    return saved;
  });
}

export async function forgetRule(agentAccess: AgentDependencies, id: string): Promise<void> {
  return forgetRules(agentAccess, [id]);
}

export function forgetRules(agentAccess: AgentDependencies, ids: string[]): Promise<void> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  return owner.serializeApprovalRules(async () => {
    try {
      for (const id of ids) {
        await owner.settle(runtime.forgetApprovalRule(id));
        owner.assertCurrent();
        commitApprovalRuleForgotten(id);
      }
    } catch (error) {
      if (!owner.isCurrent()) throw error;
      await repairProjection(owner, APPROVAL_RULES_KEY);
      throw error;
    }
    await repairProjection(owner, APPROVAL_RULES_KEY);
    owner.assertCurrent();
  });
}

function commitApprovalRuleForgotten(id: string): void {
  queryClient.setQueriesData<ApprovalRuleSummary[]>({ queryKey: [APPROVAL_RULES_KEY] }, (current) =>
    current?.filter((rule) => rule.id !== id),
  );
}

async function repairProjection(owner: AgentCommandOwner, queryKey: string): Promise<void> {
  try {
    await owner.settle(queryClient.invalidateQueries({ queryKey: [queryKey] }));
  } catch (error) {
    if (!owner.isCurrent()) throw error;
  }
}

export function allowMCPTool(
  agentAccess: AgentDependencies,
  server: MCPServerID,
  name: string,
): Promise<void> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  return owner.serializeApprovalRules(async () => {
    try {
      await owner.settle(runtime.allowMCPTool(server, name));
      owner.assertCurrent();
    } finally {
      if (owner.isCurrent()) await repairProjection(owner, APPROVAL_RULES_KEY);
    }
    owner.assertCurrent();
  });
}
