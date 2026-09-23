import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { getApprovalActions } from "./useApprovalSubmit";
import type { ApprovalDecision } from "../../domain/hitl";
import { WIRE_DECISION } from "./wireDecision";
import { resumeInterrupt } from "./useInterruptResume";
import { interruptResponseIsStaged } from "./interruptResponseCoordinator";

export function submitPendingApproval(decision: ApprovalDecision): boolean {
  const sid = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sid);
  if (!entry) return false;

  const hasPendingApproval = entry.view.pendingInterrupts.some((group) =>
    group.interrupts.some((interrupt) => interrupt.kind === "approval"),
  );
  const oi = entry.view.pendingInterrupts.find((group) =>
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
    if (decision === "approved") actions.approve();
    else actions.decline();
    return true;
  }

  resumeInterrupt(
    sid,
    oi.rootRunId,
    itemId,
    { type: "approval", decision: WIRE_DECISION[decision] },
    { decision },
  );
  return true;
}
