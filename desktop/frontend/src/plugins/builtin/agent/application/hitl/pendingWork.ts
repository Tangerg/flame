import type { AgentInterrupt, AgentPendingInterruptSet } from "@/plugins/sdk";
import type { PendingInterruptKind } from "@/plugins/sdk/types/agentSessionView";
import { createDataQuery } from "@/plugins/sdk";

export const PENDING_WORK_KEY = "pendingWork";

export interface PendingWorkItem {
  id: string;
  sessionId: string;
  rootRunId: string;
  kind: PendingInterruptKind;
  subject: string;
  more: number;
  waitingSince: string;
}

function subjectOf(interrupt: AgentInterrupt): string {
  return interrupt.type === "question"
    ? (interrupt.payload.question.fields[0]?.prompt ?? "")
    : interrupt.payload.tool.name;
}

export function pendingWorkItems(sets: readonly AgentPendingInterruptSet[]): PendingWorkItem[] {
  const items: PendingWorkItem[] = [];
  for (const set of sets) {
    const first = set.interrupts[0];
    if (!first) continue;
    items.push({
      id: `${set.sessionId}:${set.rootRunId}`,
      sessionId: set.sessionId,
      rootRunId: set.rootRunId,
      kind: first.type,
      subject: subjectOf(first),
      more: set.interrupts.length - 1,
      waitingSince: set.createdAt,
    });
  }
  return items;
}

export const usePendingWork = createDataQuery<PendingWorkItem[]>(PENDING_WORK_KEY);
