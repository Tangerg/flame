import { type AgentSessionView, type PendingInterrupt } from "@/features/agent/public/sessionView";
interface AwaitingInterruptGroup {
  runId: string;
  rootRunId: string;
  interrupts: readonly PendingInterrupt[];
}

/**
 * The interrupt groups a person can answer now. An interrupt is open only while
 * its whole tree is parked, so a group whose root no longer waits answers
 * nothing even before a refresh removes it.
 */
export function selectAwaitingGroups(view: AgentSessionView): AwaitingInterruptGroup[] {
  const awaiting: AwaitingInterruptGroup[] = [];
  for (const group of view.pendingInterrupts) {
    const rootRunId = view.runsById[group.runId]?.rootRunId;
    if (rootRunId === undefined || view.runsById[rootRunId]?.status !== "waiting") continue;
    awaiting.push({ runId: group.runId, rootRunId, interrupts: group.interrupts });
  }
  return awaiting;
}

export function selectAwaitingInterrupts(view: AgentSessionView): ReadonlyMap<string, string> {
  const awaiting = new Map<string, string>();
  for (const group of selectAwaitingGroups(view)) {
    for (const interrupt of group.interrupts) awaiting.set(interrupt.itemId, group.rootRunId);
  }
  return awaiting;
}
