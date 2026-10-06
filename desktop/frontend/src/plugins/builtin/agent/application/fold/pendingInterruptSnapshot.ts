import type { AgentPendingInterruptSet } from "@/plugins/sdk";
import type { AgentSessionView, PendingInterrupt } from "@/plugins/sdk/types/agentSessionView";
import { mergeRunPendingInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";

export function foldPendingInterruptSet(
  state: AgentSessionView,
  snapshot: AgentPendingInterruptSet,
): AgentSessionView {
  let next = state;
  const byRunId = new Map<string, PendingInterrupt[]>();
  for (const interrupt of snapshot.interrupts) {
    const current = byRunId.get(interrupt.runId) ?? [];
    current.push({ itemId: interrupt.itemId, kind: interrupt.type });
    byRunId.set(interrupt.runId, current);
  }

  for (const [runId, interrupts] of byRunId) {
    next = mergeRunPendingInterrupts(next, runId, interrupts);
  }
  for (const interrupt of snapshot.interrupts) {
    next = materializeInterrupt(next, interrupt, {
      runId: interrupt.runId,
      segmentId: null,
      eventId: `snapshot:${snapshot.rootRunId}:interrupt:${interrupt.itemId}`,
      timestamp: snapshot.createdAt,
    });
  }
  return next;
}
