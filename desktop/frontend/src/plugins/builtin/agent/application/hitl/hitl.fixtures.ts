import type { AgentRunView, PendingInterruptGroup } from "@/plugins/sdk/types/agentSessionView";

/** A parked tree whose root waits and whose interrupting Runs belong to it. */
export function waitingTree(
  sessionId: string,
  rootRunId: string,
  groups: readonly PendingInterruptGroup[],
): Record<string, AgentRunView> {
  const runs: Record<string, AgentRunView> = {};
  for (const runId of new Set([rootRunId, ...groups.map((group) => group.runId)])) {
    const root = runId === rootRunId;
    runs[runId] = {
      id: runId,
      sessionId,
      parentRunId: root ? null : rootRunId,
      rootRunId,
      spawnedByItemId: root ? null : `item_spawn_${runId}`,
      status: "waiting",
      activeSegmentId: null,
      outcome: null,
      metrics: {
        steps: 0,
        activeDurationMillis: 0,
        usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
      },
      progress: null,
      contextTokens: null,
      createdAt: "2026-07-30T00:00:00.000Z",
      finishedAt: null,
    };
  }
  return runs;
}
