import { describe, expect, it } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentRunView,
  type AgentSessionView,
  type Message,
  type ToolCall,
} from "@/features/agent/public/sessionView";
import { type ContentBlock } from "@/features/agent/public/contentBlock";
import {
  buildTranscriptRows,
  delegatedTranscriptRows,
  EMPTY_TRANSCRIPT_ROW_CACHE,
} from "@/features/agent/application/conversation/transcriptRows";
const ROOT_RUN = "root-run";

function run(overrides: Partial<AgentRunView> & Pick<AgentRunView, "id">): AgentRunView {
  return {
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: overrides.id,
    spawnedByItemId: null,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 10,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    contextTokens: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: "2026-01-01T00:00:01.000Z",
    ...overrides,
  };
}

function tool(id: string, args = "{}"): ToolCall {
  return { id, runId: ROOT_RUN, name: "read_file", fn: "Read", args, status: "ok" };
}

function message(id: string, blocks: ContentBlock[], runId: string | null = ROOT_RUN): Message {
  return { id, runId, role: "assistant", blocks };
}

function text(body: string): ContentBlock {
  return { kind: "text", text: body, status: "complete" };
}

function toolBlock(toolCallId: string): ContentBlock {
  return { kind: "tool", toolCallId };
}

function view(options: {
  messages: Message[];
  toolCalls?: ToolCall[];
  runs?: AgentRunView[];
}): AgentSessionView {
  const runs = [run({ id: ROOT_RUN }), ...(options.runs ?? [])];
  return {
    ...EMPTY_AGENT_SESSION_VIEW,
    messages: options.messages,
    runsById: Object.fromEntries(runs.map((item) => [item.id, item])),
    toolCalls: Object.fromEntries((options.toolCalls ?? []).map((item) => [item.id, item])),
  };
}

describe("transcript rows", () => {
  it("gives a selected child its own Run and message facts, excluding an active sibling", () => {
    const child = run({
      id: "child",
      parentRunId: ROOT_RUN,
      rootRunId: ROOT_RUN,
      spawnedByItemId: "delegate",
    });
    const prompt: Message = {
      ...message("child-input", [text("Check the module graph")], child.id),
      role: "user",
      steer: { runId: child.id, receipt: "accepted" },
    };
    const answer = message("child-answer", [toolBlock("read"), text("Graph verified")], child.id);
    const read = { ...tool("read"), runId: child.id };
    const projected = delegatedTranscriptRows(
      { run: child, messages: [prompt, answer] },
      {
        toolCalls: { read, sibling: { ...tool("sibling"), status: "running" } },
        delegatedRuns: { delegate: [{ run: child, messages: [prompt, answer] }] },
        awaiting: new Map([["sibling", ROOT_RUN]]),
      },
    );
    expect(projected[0]?.inputFeedback).toBe("applied");
    expect(projected[1]?.runOwner).toEqual({ kind: "owned", runId: child.id, status: "finished" });
    expect(projected[1]?.facts).toEqual({
      toolCalls: { read },
      delegatedRuns: {},
      awaiting: new Map(),
    });
  });

  it("projects descendant approvals through the root's Runtime resume barrier", () => {
    const root = run({ id: ROOT_RUN, status: "waiting", outcome: null, finishedAt: null });
    const child = run({
      id: "child-run",
      parentRunId: ROOT_RUN,
      rootRunId: ROOT_RUN,
      spawnedByItemId: "delegate",
      status: "waiting",
      outcome: null,
      finishedAt: null,
    });
    const state = view({
      runs: [root, child],
      toolCalls: [tool("delegate")],
      messages: [
        message("root-message", [toolBlock("delegate")]),
        message(
          "child-message",
          [{ kind: "approval", itemId: "approval", command: "pwd", reason: "Verify" }],
          child.id,
        ),
      ],
    });
    state.pendingInterrupts = [
      { runId: child.id, interrupts: [{ itemId: "approval", kind: "approval" }] },
    ];
    const parked = buildTranscriptRows(state, EMPTY_TRANSCRIPT_ROW_CACHE);
    expect(parked.rows[0]!.facts.awaiting.get("approval")).toBe(ROOT_RUN);
    const active = buildTranscriptRows(
      {
        ...state,
        runsById: {
          ...state.runsById,
          [ROOT_RUN]: { ...root, status: "running", activeSegmentId: "resumed" },
        },
      },
      parked.cache,
    );
    expect(active.rows[0]!.facts.awaiting.has("approval")).toBe(false);
    expect(active.rows[0]).not.toBe(parked.rows[0]);
  });

  it("keeps every untouched row identical when the tail streams", () => {
    const first = message("m1", [text("hello")]);
    const second = message("m2", [text("answer")]);
    const before = buildTranscriptRows(
      view({ messages: [first, second] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    const grown = message("m2", [text("answer, continued")]);
    const after = buildTranscriptRows(view({ messages: [first, grown] }), before.cache);

    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.rows[1]).not.toBe(before.rows[1]);
    expect(after.rows[1]?.message).toBe(grown);
  });

  it("invalidates a row only when its exact Run crosses a lifecycle boundary", () => {
    const turn = message("m1", [text("temporarily complete")]);
    const running = run({
      id: ROOT_RUN,
      status: "running",
      activeSegmentId: "segment-1",
      outcome: null,
      finishedAt: null,
    });
    const before = buildTranscriptRows(
      view({ messages: [turn], runs: [running] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(before.rows[0]?.runOwner).toEqual({
      kind: "owned",
      runId: ROOT_RUN,
      status: "running",
    });

    const progressOnly = buildTranscriptRows(
      view({
        messages: [turn],
        runs: [
          {
            ...running,
            progress: { activity: "still streaming" },
            contextTokens: null,
          },
        ],
      }),
      before.cache,
    );
    expect(progressOnly.rows[0]).toBe(before.rows[0]);

    const finished = buildTranscriptRows(view({ messages: [turn] }), progressOnly.cache);
    expect(finished.rows[0]).not.toBe(before.rows[0]);
    expect(finished.rows[0]?.runOwner).toEqual({
      kind: "owned",
      runId: ROOT_RUN,
      status: "finished",
    });
  });

  it("keeps an optimistic turn explicitly unassigned until its durable Run arrives", () => {
    const build = buildTranscriptRows(
      view({
        messages: [message("local", [text("draft")], null)],
      }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(build.rows[0]?.runOwner).toEqual({ kind: "unassigned" });
  });

  it("leaves a turn showing no tool call alone when a tool streams its arguments", () => {
    const prose = message("m1", [text("hello")]);
    const withTool = message("m2", [toolBlock("t1")]);
    const messages = [prose, withTool];
    const before = buildTranscriptRows(
      view({ messages, toolCalls: [tool("t1")] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    const after = buildTranscriptRows(
      view({ messages, toolCalls: [tool("t1", '{"path":"a.ts"}')] }),
      before.cache,
    );

    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.rows[1]).not.toBe(before.rows[1]);
    expect(after.rows[1]?.facts.toolCalls.t1?.args).toBe('{"path":"a.ts"}');
  });

  it("gives a turn only the calls it shows", () => {
    const build = buildTranscriptRows(
      view({
        messages: [message("m1", [toolBlock("t1")]), message("m2", [toolBlock("t2")])],
        toolCalls: [tool("t1"), tool("t2")],
      }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(Object.keys(build.rows[0]?.facts.toolCalls ?? {})).toEqual(["t1"]);
    expect(Object.keys(build.rows[1]?.facts.toolCalls ?? {})).toEqual(["t2"]);
  });

  it("reaches through delegation transitively, and re-slices when a subagent's turn grows", () => {
    const child = run({
      id: "child-run",
      parentRunId: ROOT_RUN,
      rootRunId: ROOT_RUN,
      spawnedByItemId: "t1",
    });
    const grandchild = run({
      id: "grandchild-run",
      parentRunId: "child-run",
      rootRunId: ROOT_RUN,
      spawnedByItemId: "t2",
    });
    const childTurn = message("child-1", [toolBlock("t2")], "child-run");
    const root = message("m1", [toolBlock("t1")]);
    const options = {
      toolCalls: [tool("t1"), tool("t2")],
      runs: [child, grandchild],
    };

    const before = buildTranscriptRows(
      view({ ...options, messages: [root, childTurn] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(before.rows).toHaveLength(1);
    expect(Object.keys(before.rows[0]?.facts.delegatedRuns ?? {}).sort()).toEqual(["t1", "t2"]);

    const grownChildTurn = message("child-1", [toolBlock("t2"), text("done")], "child-run");
    const after = buildTranscriptRows(
      view({ ...options, messages: [root, grownChildTurn] }),
      before.cache,
    );
    expect(after.rows[0]).not.toBe(before.rows[0]);
  });

  it("shares one empty facts object across turns that show nothing", () => {
    const build = buildTranscriptRows(
      view({ messages: [message("m1", [text("a")]), message("m2", [text("b")])] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(build.rows[0]?.facts).toBe(build.rows[1]?.facts);
  });

  it("drops a turn that left the transcript instead of pinning it in the cache", () => {
    const kept = message("m1", [text("a")]);
    const before = buildTranscriptRows(
      view({ messages: [kept, message("m2", [text("b")])] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );
    const after = buildTranscriptRows(view({ messages: [kept] }), before.cache);

    expect(after.rows).toHaveLength(1);
    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.cache.has("m2")).toBe(false);
  });
  it("does not infer an absent Item from a separate terminal Run observation", () => {
    const accepted: Message = {
      id: "item_steer",
      role: "user",
      runId: null,
      blocks: [text("accepted input")],
      steer: { runId: ROOT_RUN, receipt: "accepted" },
    };
    const active = view({ messages: [accepted], runs: [run({ id: ROOT_RUN, status: "running" })] });
    const before = buildTranscriptRows(active, EMPTY_TRANSCRIPT_ROW_CACHE);
    expect(before.rows[0]?.inputFeedback).toBe("accepted");
    const after = buildTranscriptRows(
      { ...active, runsById: { [ROOT_RUN]: run({ id: ROOT_RUN }) } },
      before.cache,
    );
    expect(after.rows[0]?.message).toBe(accepted);
    expect(after.rows[0]?.inputFeedback).toBe("accepted");
    expect(after.rows[0]).toBe(before.rows[0]);
  });
});
