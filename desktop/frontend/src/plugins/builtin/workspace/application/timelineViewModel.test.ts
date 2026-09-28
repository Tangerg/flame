import { describe, expect, it } from "vitest";
import type { AgentRunFact } from "@/plugins/sdk";
import type { TrajectoryEntry } from "@/plugins/builtin/agent/public/run";
import { elapsedMillis, timelineViewModel, type TimelineFilters } from "./timelineViewModel";

const filters: TimelineFilters = { category: "all", query: "", runId: null };
const startedAt = "2026-09-14T01:00:00Z";

const run: AgentRunFact = {
  id: "run_root",
  sessionId: "session_one",
  parentRunId: null,
  rootRunId: "run_root",
  spawnedByItemId: null,
  status: "finished",
  activeSegmentId: null,
  outcome: { type: "completed" },
  metrics: {
    steps: 2,
    activeDurationMillis: 90,
    usage: { inputTokens: 10, outputTokens: 20, cacheReadTokens: 0 },
  },
  createdAt: startedAt,
  finishedAt: "2026-09-14T01:00:10Z",
};

const entries: TrajectoryEntry[] = [
  {
    type: "item",
    occurredAt: startedAt,
    item: {
      type: "toolCall",
      id: "tool_no_match",
      runId: "run_child",
      status: "completed",
      startedAt,
      durationMillis: 0,
      tool: {
        name: "shell",
        arguments: { command: "rg missing" },
        result: { exitCode: 1, stdout: "needle-result" },
      },
    },
  },
  {
    type: "model",
    occurredAt: startedAt,
    model: {
      callId: "model_unknown",
      runId: "run_child",
      segmentId: "segment_child",
      state: "unknown",
      startedAt,
      settledAt: "2026-09-14T02:00:00Z",
    },
  },
  {
    type: "item",
    occurredAt: startedAt,
    item: {
      type: "agentMessage",
      id: "output",
      runId: "run_root",
      status: "completed",
      createdAt: startedAt,
      content: [{ type: "text", text: "The answer" }],
    },
  },
  { type: "run", occurredAt: startedAt, run },
];

describe("durable timeline projection", () => {
  it("preserves the Runtime's total ordering across concurrent Runs and equal timestamps", () => {
    const view = timelineViewModel(entries, filters);
    expect(view.records.map((record) => record.key)).toEqual([
      "item:tool_no_match",
      "model:model_unknown",
      "item:output",
      "run:run_root",
    ]);
    expect(view).toMatchObject({
      recordCount: 4,
      modelCount: 1,
      toolCount: 1,
      attentionCount: 1,
      runIds: ["run_child", "run_root"],
    });
    expect(view.records[0]).toMatchObject({ tone: "neutral", attention: false, durationMillis: 0 });
    expect(view.records[1]?.durationMillis).toBeUndefined();
    expect(view.records[3]?.durationMillis).toBe(10_000);
  });

  it("filters within the current page without hiding its coverage or Run identities", () => {
    const view = timelineViewModel(entries, {
      category: "toolCall",
      query: "NEEDLE-result",
      runId: "run_child",
    });
    expect(view.records.map((record) => record.id)).toEqual(["tool_no_match"]);
    expect(view.recordCount).toBe(4);
    expect(view.runIds).toEqual(["run_child", "run_root"]);
    expect(
      timelineViewModel(entries, { ...filters, category: "attention" }).records.map(
        (record) => record.id,
      ),
    ).toEqual(["model_unknown"]);
    expect(
      timelineViewModel(entries, { ...filters, query: "segment_child" }).records.map(
        (record) => record.id,
      ),
    ).toEqual(["model_unknown"]);
    expect(
      timelineViewModel(entries, { ...filters, category: "message" }).records.map(
        (record) => record.id,
      ),
    ).toEqual(["output"]);
  });

  it("distinguishes declined and incomplete tools from recorded tool errors", () => {
    const values = [
      { approvalDecision: "declined" as const },
      { status: "incomplete" as const },
      { error: { message: "connection failed" } },
    ].map((patch, index): TrajectoryEntry => ({
      type: "item",
      occurredAt: startedAt,
      item: {
        type: "toolCall",
        id: `tool_${index}`,
        runId: run.id,
        startedAt,
        status: "completed",
        tool: { name: "shell", arguments: {} },
        ...patch,
      },
    }));
    expect(
      timelineViewModel(values, filters).records.map((record) => [
        record.statusKey,
        record.tone,
        record.attention,
      ]),
    ).toEqual([
      ["timeline.state.declined", "warning", true],
      ["timeline.state.incomplete", "warning", true],
      ["timeline.modelCall.failed", "negative", true],
    ]);
  });

  it("does not search redacted reasoning or image payloads", () => {
    const values: TrajectoryEntry[] = [
      {
        type: "item",
        occurredAt: startedAt,
        item: {
          type: "reasoning",
          id: "hidden",
          runId: run.id,
          createdAt: startedAt,
          status: "completed",
          redacted: true,
          text: "hidden-reasoning",
        },
      },
      {
        type: "item",
        occurredAt: startedAt,
        item: {
          type: "userMessage",
          id: "image",
          runId: run.id,
          createdAt: startedAt,
          status: "completed",
          content: [{ type: "image", mime: "image/png", data: "hidden-image" }],
        },
      },
    ];
    expect(timelineViewModel(values, { ...filters, query: "hidden-reasoning" }).records).toEqual(
      [],
    );
    expect(timelineViewModel(values, { ...filters, query: "hidden-image" }).records).toEqual([]);
  });
});

describe("measured durations", () => {
  it("keeps zero measured while rejecting missing and invalid clock intervals", () => {
    expect(elapsedMillis(startedAt, startedAt)).toBe(0);
    expect(elapsedMillis(startedAt, undefined)).toBeUndefined();
    expect(elapsedMillis("invalid", startedAt)).toBeUndefined();
    expect(elapsedMillis(startedAt, "2026-09-14T00:59:59Z")).toBeUndefined();
  });
});
