// Convergence: streaming, history replay, and any interleaving of the two render the SAME
// turn. There are not three rendering paths — there is one fold, fed different SUBSETS of one
// event stream. Replay is the item.completed-only subset, since items.list emits no run or
// delta frames.
//
// It holds only for payload carried identically on the completed snapshot and the delta
// stream. Text and reasoning qualify: the snapshot equals the concatenated deltas. A tool's
// arguments arrive twice — parsed on the Item and again as a whole toolArguments delta for
// live preview — and the structured object at the terminal state is authoritative, so the
// redundant delta cannot make streaming diverge. Command output is the same shape: the delta
// is preview only, `{output, exitCode}` on the completed result is what replay recovers.

import { beforeEach, describe, expect, it } from "vitest";
import type {
  AgentItem as Item,
  AgentItemDelta,
  AgentStreamEvent as StreamEvent,
} from "@/plugins/sdk";
import type { AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { appendToTurn } from "./fold";
import { itemStartedAt } from "./projections";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

type StartableItem = Extract<Item, { type: "agentMessage" | "reasoning" | "toolCall" }>;

const started = (item: StartableItem): StreamEvent => {
  switch (item.type) {
    case "agentMessage":
      return {
        type: "item.started",
        item: {
          type: item.type,
          id: item.id,
          runId: item.runId,
          createdAt: item.createdAt,
          status: "running",
        },
      };
    case "reasoning":
      return {
        type: "item.started",
        item: {
          type: item.type,
          id: item.id,
          runId: item.runId,
          createdAt: item.createdAt,
          status: "running",
        },
      };
    case "toolCall":
      return {
        type: "item.started",
        item: {
          type: item.type,
          id: item.id,
          runId: item.runId,
          startedAt: item.startedAt,
          status: "running",
          tool: { name: item.tool.name, arguments: item.tool.arguments },
        },
      };
  }
};
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, value: AgentItemDelta): StreamEvent => ({
  type: "item.delta",
  itemId,
  delta: value,
});

const foldAll = (events: StreamEvent[]): AgentSessionView =>
  events.reduce((state, ev) => reduce(state, ev), EMPTY_AGENT_SESSION_VIEW);

// An assistant turn's `createdAt` is wall-clock-stamped when the turn opens (not
// event data), so compare renders without it.
const strip = (msgs: Message[]) => msgs.map(({ id, role, blocks }) => ({ id, role, blocks }));

// id of the Item an item.* event concerns (for the "snapshot this item" filter).
function itemIdOf(e: StreamEvent): string | null {
  if (e.type === "item.started" || e.type === "item.completed") return e.item.id;
  if (e.type === "item.delta") return e.itemId;
  return null;
}

// Drop the started/delta events for the given item ids — i.e. deliver those
// items as completed-only snapshots while the rest stay fully streamed.
function snapshotOnly(events: StreamEvent[], ids: Set<string>): StreamEvent[] {
  return events.filter((e) => {
    const id = itemIdOf(e);
    if (id !== null && ids.has(id) && (e.type === "item.started" || e.type === "item.delta")) {
      return false;
    }
    return true;
  });
}

// One believable turn: user prompt → reasoning → message → tool → message,
// expressed as a FULL streaming sequence. text/reasoning stream via deltas that
// concatenate to the completed snapshot; the tool is call-and-result — its args
// arrive as the parsed object AND a redundant whole toolArguments delta (as the
// real backend sends), and its result whole on completion.
const u1: Extract<Item, { type: "userMessage" }> = {
  id: "u1",
  runId: "run_1",
  type: "userMessage",
  status: "completed",
  createdAt: "2026-06-03T00:00:00Z",
  content: [{ type: "text", text: "delete it" }],
};
const r1: Extract<Item, { type: "reasoning" }> = {
  id: "r1",
  runId: "run_1",
  type: "reasoning",
  status: "completed",
  createdAt: "2026-06-03T00:00:00Z",
  text: "Weighing the risk carefully.",
};
const m1: Extract<Item, { type: "agentMessage" }> = {
  id: "m1",
  runId: "run_1",
  type: "agentMessage",
  status: "completed",
  createdAt: "2026-06-03T00:00:00Z",
  phase: "commentary",
  content: [{ type: "text", text: "Removing the file." }],
};
const t1: Extract<Item, { type: "toolCall" }> = {
  id: "t1",
  runId: "run_1",
  type: "toolCall",
  status: "completed",
  startedAt: "2026-06-03T00:00:00Z",
  tool: { name: "fs.delete", arguments: { path: "x" }, result: "ok" },
};
const t2: Extract<Item, { type: "toolCall" }> = {
  id: "t2",
  runId: "run_1",
  type: "toolCall",
  status: "completed",
  startedAt: "2026-06-03T00:00:00Z",
  tool: {
    name: "shell",
    arguments: { command: "rm x" },
    result: { output: "removed\n", exitCode: 0 },
  },
};
const m2: Extract<Item, { type: "agentMessage" }> = {
  id: "m2",
  runId: "run_1",
  type: "agentMessage",
  status: "completed",
  createdAt: "2026-06-03T00:00:00Z",
  phase: "finalAnswer",
  content: [{ type: "text", text: "Done." }],
};

const FULL_STREAM: StreamEvent[] = [
  { type: "segment.started", run: { id: "run_1", sessionId: "ses_1" } as never },
  completed(u1),
  started(r1),
  delta("r1", { type: "reasoning", text: "Weighing the risk " }),
  delta("r1", { type: "reasoning", text: "carefully." }),
  completed(r1),
  started(m1),
  delta("m1", { type: "content", text: "Removing " }),
  delta("m1", { type: "content", text: "the file." }),
  completed(m1),
  started(t1),
  delta("t1", { type: "toolArguments", argumentsTextDelta: '{"path":"x"}' }),
  completed(t1),
  started(t2),
  delta("t2", { type: "toolArguments", argumentsTextDelta: '{"command":"rm x"}' }),
  delta("t2", { type: "toolOutput", text: "removed\n" }),
  completed(t2),
  started(m2),
  delta("m2", { type: "content", text: "Done." }),
  completed(m2),
  {
    type: "segment.finished",
    contextTokens: 0,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 0,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
  },
];

describe("reducer — render convergence across delivery modes", () => {
  it("streaming, replay, and mixed delivery all fold to the same turn", () => {
    const streaming = foldAll(FULL_STREAM);

    // History replay: the item.completed-only subset (no run.* / started / delta).
    const replay = foldAll(FULL_STREAM.filter((e) => e.type === "item.completed"));

    // Mixed: m1 + both tools arrive as completed snapshots, the rest stream live.
    const mixed = foldAll(snapshotOnly(FULL_STREAM, new Set(["m1", "t1", "t2"])));

    // Same bubbles, same blocks, same order, same content.
    expect(strip(replay.messages)).toEqual(strip(streaming.messages));
    expect(strip(mixed.messages)).toEqual(strip(streaming.messages));

    // Same tool-call projections (the blocks reference these by id).
    expect(replay.toolCalls).toEqual(streaming.toolCalls);
    expect(mixed.toolCalls).toEqual(streaming.toolCalls);

    // The synthesized turn is dated by the Item that opened it — the runtime's
    // clock, the same one every message beside it carries. The fold reads no
    // clock of its own.
    expect(streaming.messages[1]!.createdAt).toBe(itemStartedAt(r1));

    // Work stays one narrated assistant turn; the Runtime-authored final answer
    // becomes its own stable message so it alone owns terminal message actions.
    expect(streaming.messages).toHaveLength(3);
    expect(streaming.messages[0]!.role).toBe("user");
    expect(streaming.messages[1]!.blocks.map((b) => b.kind)).toEqual([
      "reasoning",
      "text",
      "tool",
      "tool",
    ]);
    expect(streaming.toolCalls.t2).toMatchObject({ result: "removed\n", exitCode: 0 });
    expect(streaming.messages[1]!.phase).toBe("commentary");
    expect(streaming.messages[2]).toMatchObject({
      id: "final:m2",
      role: "assistant",
      phase: "finalAnswer",
      runId: "run_1",
      blocks: [{ kind: "text", itemId: "m2", text: "Done.", status: "complete" }],
    });
  });
});

describe("fold — one message per id", () => {
  it("re-adopts an item's turn instead of minting its id twice", () => {
    // A user message (a send, or a mid-run steer) closes the open turn. If a
    // later block for the SAME item comes back, the fold must land in the turn it
    // already minted for that item — two messages under one React key is the
    // duplicate-key loop CLAUDE.md §5 names.
    const first = appendToTurn(EMPTY_AGENT_SESSION_VIEW, "run_1", "m9", {
      kind: "text",
      text: "a",
      status: "running",
    });
    const closed: AgentSessionView = { ...first, assistantTurnByRunId: {} };
    const second = appendToTurn(closed, "run_1", "m9", {
      kind: "reasoning",
      reasoningId: "m9",
      text: "b",
      status: "running",
    });

    const ids = second.messages.map((m) => m.id);
    expect(ids).toEqual(["turn:m9"]);
    expect(second.assistantTurnByRunId.run_1).toBe("turn:m9");
    expect(second.messages[0]!.blocks.map((b) => b.kind)).toEqual(["text", "reasoning"]);
  });
});
