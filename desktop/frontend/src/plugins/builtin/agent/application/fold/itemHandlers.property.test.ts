import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { validateWire } from "@flame/runtime-contract/validate";
import type { AgentItem, AgentStreamEvent } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import { foldTestEvent, runFinished } from "./reducer.fixtures";

// The live path is not the durable one. `applyStreamHandlers` CATCHES what a handler throws
// and reports it, so a fuzz that only asserts "did not throw" here is vacuous — the failure
// mode is a silently dropped frame, a block that never appears. So the throw is observed at
// the seam that swallows it, and the ordering is the adversary: `item.started` is documented
// to arrive after deltas, after `item.completed`, after an interrupt materialised the same
// Item, or after a durable snapshot already advanced it.

const RUN_ID = "run_1";
const SEGMENT_ID = "seg_1";

function textItem(a: Arbitrary, id: string, status: "running" | "completed"): AgentItem {
  return {
    id,
    runId: RUN_ID,
    status,
    type: "agentMessage",
    createdAt: "2026-06-03T00:00:00.000Z",
    content: [{ type: "text", text: a.text() }],
    ...(status === "running" ? {} : { phase: a.pick(["commentary", "finalAnswer"]) }),
  } as AgentItem;
}

function toolItem(a: Arbitrary, id: string, status: "running" | "completed"): AgentItem {
  return {
    id,
    runId: RUN_ID,
    status,
    type: "toolCall",
    startedAt: "2026-06-03T00:00:00.000Z",
    ...(status === "running" ? {} : { finishedAt: "2026-06-03T00:00:01.000Z" }),
    tool: { name: a.pick(["read", "shell", "grep"]), arguments: { path: a.text() } },
  } as AgentItem;
}

function openSegment(): AgentStreamEvent {
  return { type: "segment.started", run: { id: RUN_ID, sessionId: "sess_1" } as never };
}

function framesFor(a: Arbitrary, id: string): AgentStreamEvent[] {
  const build = a.bool() ? textItem : toolItem;
  const started = { type: "item.started", item: build(a, id, "running") } as AgentStreamEvent;
  const completed = { type: "item.completed", item: build(a, id, "completed") } as AgentStreamEvent;
  return [started, completed];
}

function shuffled<T>(a: Arbitrary, values: readonly T[]): T[] {
  const out = [...values];
  for (let i = out.length - 1; i > 0; i -= 1) {
    const j = a.int(i + 1);
    [out[i], out[j]] = [out[j]!, out[i]!];
  }
  return out;
}

function permitted(events: readonly AgentStreamEvent[]): AgentStreamEvent[] {
  return events.filter((event) => {
    if (event.type !== "item.started" && event.type !== "item.completed") return true;
    return validateWire("Item", (event as { item: unknown }).item).length === 0;
  });
}

function foldAll(events: readonly AgentStreamEvent[]): AgentSessionView {
  let view = foldTestEvent(EMPTY_AGENT_SESSION_VIEW, openSegment(), RUN_ID, SEGMENT_ID);
  for (const event of events) view = foldTestEvent(view, event, RUN_ID, SEGMENT_ID);
  return view;
}

let swallowed: string[] = [];

// The stream handlers are a PLUGIN contribution. Without loading it `lookupStreamHandlers`
// answers empty, every frame is a no-op, and every property below holds over a view that
// was never written to — which is exactly what the non-vacuity guard caught.
beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
  swallowed = [];
  vi.spyOn(console, "error").mockImplementation((...args: unknown[]) => {
    const first = String(args[0] ?? "");
    if (first.includes("stream handler")) swallowed.push(args.map(String).join(" "));
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the live item fold, over the orderings a replay can produce", () => {
  it("actually folds the shuffled corpus, so the properties below are not measuring nothing", () => {
    let kept = 0;
    let generated = 0;
    let projected = 0;
    forEachSeed(200, (a) => {
      const raw = Array.from({ length: 2 + a.int(3) }, (_, i) => framesFor(a, `item_${i}`)).flat();
      generated += raw.length;
      const frames = permitted(shuffled(a, raw));
      kept += frames.length;
      const view = foldAll(frames);
      projected += view.messages.length + Object.keys(view.toolCalls).length;
    });
    expect(kept / generated).toBeGreaterThan(0.9);
    expect(projected).toBeGreaterThan(200);
    expect(swallowed).toEqual([]);
  });

  it("never drops a frame into the handler's catch, in any order", () => {
    forEachSeed(300, (a) => {
      swallowed = [];
      const frames = permitted(
        shuffled(
          a,
          Array.from({ length: 2 + a.int(3) }, (_, i) => framesFor(a, `item_${i}`)).flat(),
        ),
      );
      foldAll(frames);
      expect(swallowed.slice(0, 2)).toEqual([]);
    });
  });

  it("survives a frame delivered twice, which is what a replay is", () => {
    forEachSeed(300, (a) => {
      swallowed = [];
      const frames = permitted(framesFor(a, "item_0"));
      const replayed = [...frames, ...frames];
      const once = foldAll(frames);
      const twice = foldAll(replayed);
      expect(swallowed.slice(0, 2)).toEqual([]);
      expect(twice.messages.map((message) => message.id)).toEqual(
        once.messages.map((message) => message.id),
      );
      expect(Object.keys(twice.toolCalls).sort()).toEqual(Object.keys(once.toolCalls).sort());
    });
  });

  it("gives every message and every timeline entry a distinct key", () => {
    forEachSeed(300, (a) => {
      const frames = permitted(
        shuffled(
          a,
          Array.from({ length: 2 + a.int(3) }, (_, i) => framesFor(a, `item_${i}`)).flat(),
        ),
      );
      const view = foldAll([...frames, ...frames]);
      const messageIds = view.messages.map((message) => message.id);
      const timelineIds = view.timeline.map((entry) => entry.id);
      expect(new Set(messageIds).size).toBe(messageIds.length);
      expect(new Set(timelineIds).size).toBe(timelineIds.length);
    });
  });

  it("never shows a tool it was not given, whatever the order", () => {
    forEachSeed(300, (a) => {
      const ids = Array.from({ length: 2 + a.int(3) }, (_, i) => `item_${i}`);
      const frames = permitted(shuffled(a, ids.map((id) => framesFor(a, id)).flat()));
      const view = foldAll(frames);
      for (const key of Object.keys(view.toolCalls)) expect(ids).toContain(key);
    });
  });

  it("keeps a completed item completed when its start replays afterwards", () => {
    forEachSeed(300, (a) => {
      swallowed = [];
      const [started, completed] = permitted(framesFor(a, "item_0"));
      if (!started || !completed) return;
      const settled = foldAll([started, completed]);
      const regressed = foldAll([started, completed, started]);
      expect(swallowed.slice(0, 2)).toEqual([]);
      // The documented reason `item.started` refuses to upsert an advanced projection: it
      // would erase content and pull a complete card back to running.
      expect(regressed.toolCalls).toEqual(settled.toolCalls);
      expect(regressed.messages.map((message) => message.blocks.length)).toEqual(
        settled.messages.map((message) => message.blocks.length),
      );
    });
  });

  it("segments a finished run without losing what it already showed", () => {
    forEachSeed(200, (a) => {
      swallowed = [];
      const frames = permitted(framesFor(a, "item_0"));
      const view = foldAll([...frames, runFinished({ type: "completed" })]);
      expect(swallowed.slice(0, 2)).toEqual([]);
      expect(view.messages.length + Object.keys(view.toolCalls).length).toBeGreaterThan(0);
    });
  });
});
