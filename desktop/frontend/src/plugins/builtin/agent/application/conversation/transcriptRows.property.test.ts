import { describe, expect, it } from "vitest";
import type { AgentItem } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import { reduceDurableItem } from "../fold/reducer";
import { buildTranscriptRows, EMPTY_TRANSCRIPT_ROW_CACHE } from "./transcriptRows";

// A cache bug here does not throw, it shows stale content. The property is the module's own
// claim: feeding the previous cache back answers what never having one would.
//
// TRAP: `runId` must name no Run the view holds, or the selector excludes every message and
// the properties below hold over an empty transcript.
function messageItem(a: Arbitrary, id: string): AgentItem {
  return {
    id,
    runId: null,
    status: "completed",
    type: "userMessage",
    createdAt: "2026-07-07T10:00:00Z",
    content: [{ type: "text", text: a.text() }],
  } as unknown as AgentItem;
}

function toolItem(a: Arbitrary, id: string, result: string): AgentItem {
  return {
    id,
    runId: null,
    status: "completed",
    type: "toolCall",
    startedAt: "2026-07-07T10:00:00Z",
    finishedAt: "2026-07-07T10:00:01Z",
    tool: { name: a.pick(["read", "shell", "grep"]), arguments: { path: a.text() }, result },
  } as unknown as AgentItem;
}

function viewOf(items: readonly AgentItem[]) {
  let view = EMPTY_AGENT_SESSION_VIEW;
  for (const item of items) view = reduceDurableItem(view, item);
  return view;
}

describe("the transcript row cache", () => {
  it("builds rows at all, so the properties below are not measuring an empty list", () => {
    forEachSeed(50, (a) => {
      const items = Array.from({ length: 3 }, (_, i) => messageItem(a, `item_${i}`));
      const built = buildTranscriptRows(viewOf(items), EMPTY_TRANSCRIPT_ROW_CACHE);
      expect(built.rows.length).toBe(3);
    });
  });

  it("answers what a cold build would, whatever it was warmed with", () => {
    forEachSeed(300, (a) => {
      const items = Array.from({ length: 1 + a.int(5) }, (_, i) => messageItem(a, `item_${i}`));
      let cache = EMPTY_TRANSCRIPT_ROW_CACHE;
      for (let step = 1; step <= items.length; step += 1) {
        const view = viewOf(items.slice(0, step));
        const warm = buildTranscriptRows(view, cache);
        const cold = buildTranscriptRows(view, EMPTY_TRANSCRIPT_ROW_CACHE);
        expect(warm.rows.map((row) => row.message.id)).toEqual(
          cold.rows.map((row) => row.message.id),
        );
        expect(warm.rows.length).toBe(cold.rows.length);
        cache = warm.cache;
      }
    });
  });

  // The cache holds ROWS; the array's identity is stabilised a layer up.
  it("hands back the very same row when nothing under it moved", () => {
    forEachSeed(300, (a) => {
      const items = Array.from({ length: 2 + a.int(4) }, (_, i) => messageItem(a, `item_${i}`));
      const view = viewOf(items);
      const first = buildTranscriptRows(view, EMPTY_TRANSCRIPT_ROW_CACHE);
      const second = buildTranscriptRows(view, first.cache);
      expect(second.rows.length).toBe(first.rows.length);
      for (const [index, row] of second.rows.entries()) expect(row).toBe(first.rows[index]);
    });
  });

  it("does not hand back a row for a turn the transcript no longer has", () => {
    forEachSeed(300, (a) => {
      const items = Array.from({ length: 2 + a.int(4) }, (_, i) => messageItem(a, `item_${i}`));
      const full = buildTranscriptRows(viewOf(items), EMPTY_TRANSCRIPT_ROW_CACHE);
      const fewer = buildTranscriptRows(viewOf(items.slice(0, 1)), full.cache);
      expect(fewer.rows.map((row) => row.message.id)).toEqual([items[0]!.id]);
      expect(fewer.cache.size).toBeLessThanOrEqual(fewer.rows.length);
    });
  });

  // Staleness is the failure this can actually have, so the row carrying a changed tool must
  // not come back as the object built before the change.
  it("rebuilds the row whose tool moved, and only that row", () => {
    forEachSeed(300, (a) => {
      // Evolved, not rebuilt: folding the same items into a fresh view makes every
      // message a new object, and the cache would then be right to rebuild all of
      // them. Production advances one item at a time, which is what this measures.
      const baseView = viewOf([messageItem(a, "item_0"), toolItem(a, "item_1", "first")]);
      const nextView = reduceDurableItem(baseView, toolItem(a, "item_1", "second"));
      const before = buildTranscriptRows(baseView, EMPTY_TRANSCRIPT_ROW_CACHE);
      const after = buildTranscriptRows(nextView, before.cache);
      const carriesTool = (row: (typeof after.rows)[number]) =>
        Object.hasOwn(row.facts.toolCalls, "item_1");
      const movedRow = after.rows.find(carriesTool);
      const stableRow = after.rows.find((row) => !carriesTool(row));
      expect(movedRow).toBeDefined();
      expect(stableRow).toBeDefined();
      expect(movedRow).not.toBe(before.rows.find(carriesTool));
      expect(stableRow).toBe(before.rows.find((row) => !carriesTool(row)));
    });
  });
});
