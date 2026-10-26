import { type AgentAccess } from "@/features/agent/public/agent";

import { getApprovalActions } from "@/features/agent/application/hitl/useApprovalSubmit";
import { type ApprovalDecision } from "@/features/agent/domain/hitl";
import { resumeInterrupt } from "@/features/agent/application/hitl/useInterruptResume";

import { selectAwaitingGroups } from "@/features/agent/application/view/awaitingInterrupts";
export function submitPendingApproval(
  agentAccess: AgentAccess,
  decision: ApprovalDecision,
): boolean {
  const sid = agentAccess.state.getActiveSessionId();
  const entry = agentAccess.view.getSession(sid);
  if (!entry) return false;

  const awaiting = selectAwaitingGroups(entry.view);
  const hasPendingApproval = awaiting.some((group) =>
    group.interrupts.some((interrupt) => interrupt.kind === "approval"),
  );
  const oi = awaiting.find((group) =>
    group.interrupts.some(
      (interrupt) =>
        interrupt.kind === "approval" &&
        !agentAccess.interrupts.isStaged({
          sessionId: sid,
          rootRunId: group.rootRunId,
          itemId: interrupt.itemId,
        }),
    ),
  );
  const interrupt = oi?.interrupts.find(
    (candidate) =>
      candidate.kind === "approval" &&
      !agentAccess.interrupts.isStaged({
        sessionId: sid,
        rootRunId: oi.rootRunId,
        itemId: candidate.itemId,
      }),
  );
  if (!oi || !interrupt) return hasPendingApproval;

  const itemId = interrupt.itemId;
  const actions = getApprovalActions({ sessionId: sid, rootRunId: oi.rootRunId, itemId });
  if (actions) {
    if (decision === "approve") actions.approve();
    else actions.decline();
    return true;
  }

  resumeInterrupt(agentAccess, sid, oi.rootRunId, itemId, {
    type: "approval",
    decision,
  });
  return true;
}
