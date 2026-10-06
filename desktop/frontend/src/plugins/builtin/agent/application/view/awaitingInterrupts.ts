import type { AgentSessionView, PendingInterruptGroup } from "@/plugins/sdk/types/agentSessionView";

/**
 * The interrupt groups a person can answer now. An interrupt is open only while
 * its whole tree is parked, so a group whose root no longer waits answers
 * nothing even before a refresh removes it.
 */
export function selectAwaitingGroups(view: AgentSessionView): PendingInterruptGroup[] {
  return view.pendingInterrupts.filter(
    (group) => view.runsById[group.rootRunId]?.status === "waiting",
  );
}

/** The Run to resume for every Item awaiting a person's answer. */
export function selectAwaitingInterrupts(view: AgentSessionView): ReadonlyMap<string, string> {
  const awaiting = new Map<string, string>();
  for (const group of selectAwaitingGroups(view)) {
    for (const interrupt of group.interrupts) awaiting.set(interrupt.itemId, group.rootRunId);
  }
  return awaiting;
}
