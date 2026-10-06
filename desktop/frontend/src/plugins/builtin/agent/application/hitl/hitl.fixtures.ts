import type { AgentRunView, PendingInterruptGroup } from "@/plugins/sdk/types/agentSessionView";

/** The parked roots that make these interrupt groups answerable. */
export function waitingRoots(
  groups: readonly PendingInterruptGroup[],
): Record<string, AgentRunView> {
  const runs: Record<string, AgentRunView> = {};
  for (const group of groups) {
    runs[group.rootRunId] = {
      id: group.rootRunId,
      sessionId: group.sessionId,
      parentRunId: null,
      rootRunId: group.rootRunId,
      spawnedByItemId: null,
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
