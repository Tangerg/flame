import { describe, expect, it } from "vitest";
import type { AgentItem } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { onItemCompleted, onItemStarted } from "./itemHandlers";
import { reduceDurableItem } from "./reducer";

const tool: Extract<AgentItem, { type: "toolCall" }> = {
  id: "item_tool",
  runId: "run_child",
  type: "toolCall",
  status: "completed",
  startedAt: "2026-09-14T01:00:00.000Z",
  finishedAt: "2026-09-14T01:02:00.000Z",
  durationMillis: 230,
  tool: { name: "shell", arguments: { command: "go test ./..." }, result: { exitCode: 0 } },
};
const source = {
  runId: tool.runId,
  segmentId: "seg_child",
  eventId: "evt_live",
  timestamp: "2026-09-14T01:05:00.000Z",
};

describe("durable item trajectory", () => {
  it("reconstructs the same tool boundaries from a terminal snapshot as from live events", () => {
    const started = onItemStarted(
      EMPTY_AGENT_SESSION_VIEW,
      { ...tool, status: "running", finishedAt: undefined, durationMillis: undefined },
      source,
    );
    const live = onItemCompleted(started, tool, { ...source, eventId: "evt_completed" });
    const restored = reduceDurableItem(EMPTY_AGENT_SESSION_VIEW, tool);
    expect(restored.timeline).toEqual(live.timeline);
    expect(restored.timeline.map(({ kind, ts }) => ({ kind, ts }))).toEqual([
      { kind: "tool-start", ts: Date.parse(tool.startedAt) },
      { kind: "tool-end", ts: Date.parse(tool.finishedAt!) },
    ]);
    expect(reduceDurableItem(live, tool).timeline).toEqual(live.timeline);
    expect(restored.toolCalls[tool.id]?.durationMillis).toBe(230);
  });

  it("does not invent an end timestamp for an incomplete tool without one", () => {
    const restored = reduceDurableItem(EMPTY_AGENT_SESSION_VIEW, {
      ...tool,
      status: "incomplete",
      finishedAt: undefined,
      durationMillis: undefined,
    });
    expect(restored.timeline.map((entry) => entry.kind)).toEqual(["tool-start"]);
  });

  it("retains compaction in both live and restored trajectories without duplicating its summary", () => {
    const compaction: AgentItem = {
      id: "item_compact",
      runId: tool.runId,
      type: "compaction",
      status: "completed",
      createdAt: tool.finishedAt!,
      summary: "Retained axios cancellation and request tests",
      droppedMessages: 12,
    };
    const live = onItemCompleted(EMPTY_AGENT_SESSION_VIEW, compaction, source);
    const restored = reduceDurableItem(EMPTY_AGENT_SESSION_VIEW, compaction);
    expect(restored.timeline).toEqual(live.timeline);
    expect(restored.timeline).toEqual([
      expect.objectContaining({
        kind: "compaction",
        refId: compaction.id,
        ts: Date.parse(compaction.createdAt),
        runId: tool.runId,
      }),
    ]);
    expect(reduceDurableItem(live, compaction).timeline).toEqual(live.timeline);
  });
});
