import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import type { AgentItem } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import { reduceDurableItem } from "./reducer";

// The reducer swallows what a projection throws, so a bad read is invisible in
// production: no error, just a block that never appears or a card nobody can
// answer. The canonical samples pin the shapes the runtime publishes today; these
// explore the space around them — empty and astral text, absent optionals, ids
// reused across variants, statuses arriving out of order — and assert the
// properties the fold has to hold whatever it is handed.

const TYPES = ["userMessage", "agentMessage", "reasoning", "toolCall", "question"] as const;
const STATUSES = ["running", "completed", "incomplete"] as const;
const TOOL_NAMES = ["read", "shell", "apply_patch", "grep", "web_search", "ask_user", ""] as const;

function item(a: Arbitrary, id: string): AgentItem {
  const type = a.pick(TYPES);
  const status = a.pick(STATUSES);
  const base = { id, runId: a.bool(0.9) ? "run_01" : null, status };
  switch (type) {
    case "userMessage":
      // A user message is atomic: the contract admits it only as completed.
      return {
        ...base,
        status: "completed",
        type,
        createdAt: "2026-07-07T10:00:00Z",
        content: a.bool(0.15) ? [] : [{ type: "text", text: a.text() }],
      } as AgentItem;
    case "agentMessage":
      // `phase` is what the provisional running shell has not decided yet, so it is
      // required on a terminal message and refused on a running one.
      return {
        ...base,
        type,
        createdAt: "2026-07-07T10:00:00Z",
        content: a.bool(0.15) ? [] : [{ type: "text", text: a.text() }],
        ...(status === "running" ? {} : { phase: a.pick(["commentary", "finalAnswer"]) }),
      } as AgentItem;
    case "reasoning":
      return { ...base, type, createdAt: "2026-07-07T10:00:00Z", text: a.text() } as AgentItem;
    case "question":
      return {
        ...base,
        status: "completed",
        type,
        createdAt: "2026-07-07T10:00:00Z",
        question: {
          fields: [{ prompt: a.text(), type: "text" }],
          ...(a.bool(0.3) ? { answers: [[a.text()]] } : {}),
        },
      } as AgentItem;
    case "toolCall":
      return {
        ...base,
        type,
        startedAt: "2026-07-07T10:00:00Z",
        ...(status === "running" ? {} : { finishedAt: "2026-07-07T10:00:01Z" }),
        tool: {
          name: a.pick(TOOL_NAMES),
          arguments: a.bool(0.8) ? { path: a.text(), command: a.text() } : {},
          ...(a.bool(0.6) ? { result: a.bool(0.5) ? a.text() : { hits: [] } } : {}),
        },
      } as AgentItem;
  }
}

// The transport refuses a frame the contract does not permit, so the fold never
// sees one. Generating outside that space would only manufacture phantom bugs, so
// anything invalid is dropped here and the suites assert they still had material.
function permitted(items: readonly AgentItem[]): AgentItem[] {
  return items.filter((next) => validateWire("Item", next).length === 0);
}

function fold(items: readonly AgentItem[]): AgentSessionView {
  let view = EMPTY_AGENT_SESSION_VIEW;
  for (const next of items) view = reduceDurableItem(view, next);
  return view;
}

describe("the fold, over the space around the published shapes", () => {
  // Filtering to what the contract permits could quietly filter to nothing, and
  // every property below would then hold over an empty corpus.
  it("explores a corpus the contract actually permits", () => {
    let kept = 0;
    let generated = 0;
    const variants = new Set<string>();
    forEachSeed(400, (a) => {
      const raw = Array.from({ length: 6 }, (_, i) => item(a, `item_${i}`));
      generated += raw.length;
      for (const next of permitted(raw)) {
        kept += 1;
        variants.add(next.type);
      }
    });
    expect(kept / generated).toBeGreaterThan(0.5);
    expect(variants).toEqual(new Set(TYPES));
  });

  it("always answers with a view", () => {
    forEachSeed(400, (a) => {
      const items = permitted(
        Array.from({ length: 1 + a.int(6) }, (_, i) => item(a, `item_${a.int(4)}${i}`)),
      );
      const view = fold(items);
      expect(view).toBeDefined();
      expect(Array.isArray(view.messages)).toBe(true);
    });
  });

  it("never invents a tool call for an id it was not given", () => {
    forEachSeed(400, (a) => {
      const items = permitted(Array.from({ length: 1 + a.int(6) }, (_, i) => item(a, `item_${i}`)));
      const ids = new Set(items.map((next) => next.id));
      for (const id of Object.keys(fold(items).toolCalls)) expect(ids.has(id)).toBe(true);
    });
  });

  it("lands in the same place when history replays the whole sequence", () => {
    forEachSeed(400, (a) => {
      const items = permitted(Array.from({ length: 1 + a.int(5) }, (_, i) => item(a, `item_${i}`)));
      const once = fold(items);
      const twice = fold([...items, ...items]);
      expect(twice.messages.length).toBe(once.messages.length);
      expect(Object.keys(twice.toolCalls).sort()).toEqual(Object.keys(once.toolCalls).sort());
    });
  });

  it("keeps every message it has already shown", () => {
    forEachSeed(400, (a) => {
      const items = permitted(Array.from({ length: 1 + a.int(6) }, (_, i) => item(a, `item_${i}`)));
      let view = EMPTY_AGENT_SESSION_VIEW;
      let seen = 0;
      for (const next of items) {
        view = reduceDurableItem(view, next);
        expect(view.messages.length).toBeGreaterThanOrEqual(seen);
        seen = view.messages.length;
      }
    });
  });

  it("gives every message a distinct React key", () => {
    forEachSeed(400, (a) => {
      const items = permitted(Array.from({ length: 1 + a.int(6) }, (_, i) => item(a, `item_${i}`)));
      const ids = fold(items).messages.map((message) => message.id);
      expect(new Set(ids).size).toBe(ids.length);
    });
  });
});
