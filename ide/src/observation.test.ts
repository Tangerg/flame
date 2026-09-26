import { describe, expect, it, vi } from "vitest";
import type { RunEvent, RunRef, SessionSnapshot } from "@flame/runtime-contract/wire";
import runRef from "@flame/runtime-contract/samples/runref.full.json";
import { observeRun } from "./observation";

const running: RunRef = {
  ...(runRef as RunRef),
  status: "running",
  activeSegmentId: "seg_current",
};
const snapshot: SessionSnapshot = { runs: [running], items: [], interrupts: [] };
const event: RunEvent = {
  runId: running.id,
  segmentId: "seg_current",
  eventId: "event_current",
  timestamp: "2026-09-26T00:00:00Z",
  event: { type: "segment.started", run: running },
};

describe("IDE run attachment", () => {
  it("observes the atomic snapshot before its tail and rereads authoritative state after tail end", async () => {
    const order: string[] = [];
    const get = vi
      .fn()
      .mockResolvedValueOnce(running)
      .mockResolvedValueOnce({ ...running, status: "waiting" });
    const subscribe = vi.fn().mockResolvedValue({
      result: { runId: running.id, segmentId: "seg_current", snapshot },
      events: (async function* () {
        yield event;
      })(),
    });
    await observeRun(
      { runs: { get, subscribe } },
      running.id,
      { snapshot: () => order.push("snapshot"), event: () => order.push("tail") },
      new AbortController().signal,
    );
    expect(subscribe.mock.calls[0]?.[0]).toEqual({
      runId: running.id,
      segmentId: "seg_current",
      snapshot: true,
    });
    expect(order).toEqual(["snapshot", "tail"]);
    expect(get).toHaveBeenCalledTimes(2);
  });

  it("closes its local subscription when snapshot delivery retires the view, without canceling the Run", async () => {
    const abort = new AbortController();
    const returned = vi.fn(async () => ({ done: true as const, value: undefined }));
    const next = vi.fn(async () => ({ done: true as const, value: undefined }));
    const subscribe = vi.fn().mockResolvedValue({
      result: { runId: running.id, segmentId: "seg_current", snapshot },
      events: { [Symbol.asyncIterator]: () => ({ next, return: returned }) },
    });
    await observeRun(
      { runs: { get: vi.fn().mockResolvedValue(running), subscribe } },
      running.id,
      {
        snapshot: () => abort.abort(),
        event: () => {
          throw new Error("retired view received an event");
        },
      },
      abort.signal,
    );
    expect(returned).toHaveBeenCalledOnce();
    expect(subscribe).toHaveBeenCalledOnce();
  });
});
