import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";
import type { AgentItem } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { reduceDurableItem } from "./reducer";

// The fold is where the runtime's shapes meet our read model, and the reducer
// swallows what a projection throws — a bad read there does not surface as an
// error, it surfaces as a block that never appears or a HITL card nobody can
// answer. These drive the runtime's OWN published samples through the real entry
// point, so the fold is checked against what the runtime says it emits rather
// than against fixtures that can only agree with whatever the fold already does.
//
// The set is derived, not listed: every sample the contract calls an Item is
// covered the day it is added, and an envelope sample cannot be mistaken for one.
const SAMPLE_PREFIX = "../../../../../../../../runtime/contract/typescript/samples/";
const files = import.meta.glob<{ default: unknown }>(
  "../../../../../../../../runtime/contract/typescript/samples/*.json",
  { eager: true },
);

const items: { name: string; item: AgentItem }[] = Object.entries(files)
  .map(([path, loaded]) => ({ name: path.slice(SAMPLE_PREFIX.length), item: loaded.default }))
  .filter(({ item }) => validateWire("Item", item).length === 0)
  .map(({ name, item }) => ({ name, item: item as AgentItem }));

describe("the fold against the runtime's own samples", () => {
  it("finds the published Item samples", () => {
    expect(items.length).toBeGreaterThan(0);
  });

  it.each(items.map(({ name }) => name))("takes %s without losing it", (name) => {
    const { item } = items.find((candidate) => candidate.name === name)!;

    const before = EMPTY_AGENT_SESSION_VIEW;
    const after = reduceDurableItem(before, item);

    expect(after).toBeDefined();
    expect(
      after.messages.length > before.messages.length ||
        Object.keys(after.toolCalls).length > Object.keys(before.toolCalls).length ||
        after.timeline.length > before.timeline.length,
    ).toBe(true);
  });

  it("folds every sample in sequence without losing an earlier one", () => {
    let view = EMPTY_AGENT_SESSION_VIEW;
    for (const { item } of items) view = reduceDurableItem(view, item);
    expect(view.messages.length).toBeGreaterThan(0);
  });

  // History hydration replays items the projection may already hold, so folding
  // one twice has to land where folding it once did.
  it.each(items.map(({ name }) => name))("is idempotent when %s replays", (name) => {
    const { item } = items.find((candidate) => candidate.name === name)!;

    const once = reduceDurableItem(EMPTY_AGENT_SESSION_VIEW, item);
    const twice = reduceDurableItem(once, item);

    expect(twice.messages.length).toBe(once.messages.length);
    expect(Object.keys(twice.toolCalls)).toEqual(Object.keys(once.toolCalls));
  });
});
