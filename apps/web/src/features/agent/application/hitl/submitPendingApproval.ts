import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type ApprovalDecision } from "@/features/agent/domain/hitl";
import { selectAwaitingGroups } from "@/features/agent/application/view/awaitingInterrupts";
export function submitPendingApproval(
  agentAccess: AgentDependencies,
  decision: ApprovalDecision,
): boolean {
  const sessionId = agentAccess.state.getActiveSessionId();
  const entry = agentAccess.view.getSession(sessionId);
  if (!entry) return false;

  let hasApproval = false;
  for (const { rootRunId, interrupts } of selectAwaitingGroups(entry.view)) {
    for (const { itemId, type } of interrupts) {
      if (type !== "approval") continue;
      hasApproval = true;
      if (agentAccess.interrupts.responseState(sessionId, itemId) !== "unsubmitted") continue;
      const ref = { sessionId, rootRunId, itemId };
      const actions = agentAccess.approvalActions.find(ref);
      if (!actions) agentAccess.interrupts.stage(ref, { type: "approval", decision });
      else if (decision === "approve") actions.approve();
      else actions.decline();
      return true;
    }
  }
  return hasApproval;
}
