import { agentSessionState } from "@/features/agent/application/ports/sessionState";
import { agentSessionView } from "@/features/agent/application/ports/sessionView";
import { getApprovalActions } from "@/features/agent/application/hitl/useApprovalSubmit";
import { type ApprovalDecision } from "@/features/agent/domain/hitl";
import { resumeInterrupt } from "@/features/agent/application/hitl/useInterruptResume";
import { interruptResponseIsStaged } from "@/features/agent/application/hitl/interruptResponseCoordinator";
import { selectAwaitingGroups } from "@/features/agent/application/view/awaitingInterrupts";
export function submitPendingApproval(decision: ApprovalDecision): boolean {
  const sid = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sid);
  if (!entry) return false;

  const awaiting = selectAwaitingGroups(entry.view);
  const hasPendingApproval = awaiting.some((group) =>
    group.interrupts.some((interrupt) => interrupt.kind === "approval"),
  );
  const oi = awaiting.find((group) =>
    group.interrupts.some(
      (interrupt) =>
        interrupt.kind === "approval" &&
        !interruptResponseIsStaged({
          sessionId: sid,
          rootRunId: group.rootRunId,
          itemId: interrupt.itemId,
        }),
    ),
  );
  const interrupt = oi?.interrupts.find(
    (candidate) =>
      candidate.kind === "approval" &&
      !interruptResponseIsStaged({
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

  resumeInterrupt(sid, oi.rootRunId, itemId, {
    type: "approval",
    decision,
  });
  return true;
}
