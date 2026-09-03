// The card-less keyboard path behind ⌘↩ / ⇧⌘⌫: answer the active session's first unstaged
// approval into the shared atomic response set. Returns true whenever an approval is
// pending OR staged, so the keybinding never falls through into chat send while the barrier
// is open. Staging, deduplication and rollback belong to the coordinator.

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

  // Questions need answers (not approve/deny), so only act on approval interrupts.
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
  // Every approval in the atomic set is already staged or submitting. Consume
  // a repeated shortcut instead of letting it fall through into chat send.
  if (!oi || !interrupt) return hasPendingApproval;

  const itemId = interrupt.itemId;
  // Prefer the mounted card's own submit so the shortcut applies its edited
  // args + remember exactly like its buttons. Direct staging below is only for
  // the no-card-mounted fallback.
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
