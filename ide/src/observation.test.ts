import { describe, expect, it, vi } from "vitest";
import type { RunEvent, RunRef, SessionSnapshot } from "@flame/runtime-contract/wire";
import runRef from "@flame/runtime-contract/samples/runref.full.json";
import sessionSample from "@flame/runtime-contract/samples/session.json";
import { observeRun } from "./observation";
import { RpcError } from "@flame/runtime-contract/client";

const running: RunRef = {
  ...(runRef as RunRef),
  status: "running",
  activeSegmentId: "seg_current",
};
const snapshot: SessionSnapshot = {
  session: sessionSample as SessionSnapshot["session"],
  runs: [running],
  items: [],
  interrupts: [],
};
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
      { runs: { get, subscribe }, sessions: { snapshot: vi.fn().mockResolvedValue(snapshot) } },
      running.id,
      { snapshot: () => order.push("snapshot"), event: () => order.push("tail") },
      new AbortController().signal,
    );
    expect(subscribe.mock.calls[0]?.[0]).toEqual({
      runId: running.id,
      segmentId: "seg_current",
      snapshot: true,
    });
    expect(order).toEqual(["snapshot", "tail", "snapshot"]);
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
      {
        runs: { get: vi.fn().mockResolvedValue(running), subscribe },
        sessions: { snapshot: vi.fn() },
      },
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

  it.each(["waiting", "finished"])(
    "refreshes durable material when the Run is already %s before attachment",
    async (status) => {
      const complete = { ...snapshot, runs: [{ ...running, status }] };
      const readSnapshot = vi.fn().mockResolvedValue(complete);
      const sink = { snapshot: vi.fn(), event: vi.fn() };
      const subscribe = vi.fn();
      await observeRun(
        {
          runs: { get: vi.fn().mockResolvedValue({ ...running, status }), subscribe },
          sessions: { snapshot: readSnapshot },
        },
        running.id,
        sink,
        new AbortController().signal,
      );
      expect(readSnapshot).toHaveBeenCalledWith(running.sessionId, true, expect.any(AbortSignal));
      expect(sink.snapshot).toHaveBeenCalledWith(complete);
      expect(subscribe).not.toHaveBeenCalled();
    },
  );

  it.each(["stale_segment", "run_waiting", "run_finished"] as const)(
    "rereads authoritative state when subscription races a %s boundary",
    async (type) => {
      const get = vi
        .fn()
        .mockResolvedValueOnce(running)
        .mockResolvedValueOnce({
          ...running,
          status: "finished",
        });
      const subscribe = vi.fn().mockRejectedValue(new RpcError({ message: type, data: { type } }));
      const sink = { snapshot: vi.fn(), event: vi.fn() };
      await observeRun(
        { runs: { get, subscribe }, sessions: { snapshot: vi.fn().mockResolvedValue(snapshot) } },
        running.id,
        sink,
        new AbortController().signal,
      );
      expect(get).toHaveBeenCalledTimes(2);
      expect(subscribe).toHaveBeenCalledOnce();
      expect(sink.snapshot).toHaveBeenCalledWith(snapshot);
    },
  );
});
