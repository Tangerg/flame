import { describe, expect, it } from "vitest";
import type { RunEvent } from "@flame/runtime-contract/client";
import { validateWire } from "@flame/runtime-contract/validate";
import segmentStarted from "@flame/runtime-contract/samples/segment.started.json";
import segmentFinished from "@flame/runtime-contract/samples/segment.finished.json";
import segmentFinishedInterrupt from "@flame/runtime-contract/samples/segment.finished.interrupt.json";
import type { AgentItem } from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { runtimeAgentEvent } from "../../adapters/runtimeAgentFacts";
import { selectAwaitingInterrupts } from "../view/awaitingInterrupts";
import { reduceAgentEvent, reduceDurableItem } from "./reducer";

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
        Object.keys(after.toolCalls).length > Object.keys(before.toolCalls).length,
    ).toBe(true);
  });

  it("folds every sample in sequence without losing an earlier one", () => {
    let view = EMPTY_AGENT_SESSION_VIEW;
    for (const { item } of items) view = reduceDurableItem(view, item);
    expect(view.messages.length).toBeGreaterThan(0);
  });

  it.each(items.map(({ name }) => name))("is idempotent when %s replays", (name) => {
    const { item } = items.find((candidate) => candidate.name === name)!;

    const once = reduceDurableItem(EMPTY_AGENT_SESSION_VIEW, item);
    const twice = reduceDurableItem(once, item);

    expect(twice.messages.length).toBe(once.messages.length);
    expect(Object.keys(twice.toolCalls)).toEqual(Object.keys(once.toolCalls));
  });
});

describe("segment.finished against the runtime's own samples", () => {
  const foldFrames = (...frames: unknown[]) =>
    frames.reduce(
      (view: typeof EMPTY_AGENT_SESSION_VIEW, frame) =>
        reduceAgentEvent(view, runtimeAgentEvent(frame as RunEvent)),
      EMPTY_AGENT_SESSION_VIEW,
    );

  it("replaces the Run with the finished Run the frame carries", () => {
    const view = foldFrames(segmentStarted, segmentFinished);

    expect(view.runsById.run_01).toMatchObject({
      status: "finished",
      activeSegmentId: null,
      outcome: { type: "completed" },
      metrics: {
        steps: 3,
        activeDurationMillis: 12_400,
        usage: { inputTokens: 1200, outputTokens: 340, costUsd: 0.021 },
      },
      contextTokens: 1200,
      progress: null,
      finishedAt: "2026-07-07T10:00:12Z",
    });
    expect(view.pendingInterrupts).toEqual([]);
  });

  it("parks the Run on the interrupts it raised itself", () => {
    const view = foldFrames(segmentStarted, segmentFinishedInterrupt);

    expect(view.runsById.run_01).toMatchObject({
      status: "waiting",
      activeSegmentId: null,
      outcome: null,
      metrics: { steps: 2, activeDurationMillis: 8100 },
      finishedAt: null,
    });
    expect(view.pendingInterrupts).toEqual([
      { runId: "run_01", interrupts: [{ itemId: "item_09", kind: "approval" }] },
    ]);
    expect(selectAwaitingInterrupts(view).get("item_09")).toBe("run_01");
    expect(
      view.messages.flatMap((message) => message.blocks).find((block) => block.kind === "approval"),
    ).toMatchObject({ itemId: "item_09", command: "rm -rf build" });
  });

  it("settles a replayed finish without a second card", () => {
    const once = foldFrames(segmentStarted, segmentFinishedInterrupt);
    const twice = reduceAgentEvent(once, runtimeAgentEvent(segmentFinishedInterrupt as RunEvent));

    expect(twice).toBe(once);
  });
});
