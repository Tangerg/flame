import { type TrajectoryEntry } from "@flame/runtime-client/wire";
import { type RuntimeAgentSessionSnapshot } from "./agentSessionSnapshots";
export function visualTrajectory(snapshot: RuntimeAgentSessionSnapshot): TrajectoryEntry[] {
  const run = snapshot.runs[0]!;
  const entries: TrajectoryEntry[] = [
    ...snapshot.runs.map((value): TrajectoryEntry => ({
      type: "run",
      occurredAt: value.createdAt,
      run: value,
    })),
    ...snapshot.items.map((item): TrajectoryEntry => ({
      type: "item",
      occurredAt: item.type === "toolCall" ? item.startedAt : item.createdAt,
      item,
    })),
    {
      type: "model",
      occurredAt: "2026-07-31T08:00:01.000Z",
      model: {
        callId: "call_visual_inspect",
        runId: run.id,
        segmentId: "segment_visual",
        state: "completed",
        startedAt: "2026-07-31T08:00:01.000Z",
        settledAt: "2026-07-31T08:00:02.420Z",
        firstOutputLatencyMillis: 320,
        usage: { inputTokens: 24_000, outputTokens: 310, cacheReadTokens: 18_000 },
      },
    },
    {
      type: "model",
      occurredAt: "2026-07-31T08:00:06.000Z",
      model: {
        callId: "call_visual_recovered",
        runId: run.id,
        segmentId: "segment_visual",
        state: "unknown",
        startedAt: "2026-07-31T08:00:06.000Z",
        settledAt: "2026-07-31T08:01:00.000Z",
      },
    },
  ];
  return entries.sort((left, right) => Date.parse(right.occurredAt) - Date.parse(left.occurredAt));
}
