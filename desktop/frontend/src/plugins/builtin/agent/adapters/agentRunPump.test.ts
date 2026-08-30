// The pump keeps a run attached, not a stream open. These lock the difference: a
// stream that ends without the segment's own terminal is a dropped connection, and
// the run behind it is still executing.

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunEvent, RunRef } from "@/rpc";
import { asRunId, asSegmentId, asSessionId, RpcConnectionError, RpcProtocolError } from "@/rpc";
import { createAgentRunPump, type RunStream, type RunStreamPosition } from "./agentRunPump";

const RUN = asRunId("run_1");
const SEGMENT = asSegmentId("seg_1");

let nextFrame = 0;

beforeEach(() => {
  vi.stubGlobal("requestAnimationFrame", () => ++nextFrame);
  vi.stubGlobal("cancelAnimationFrame", () => undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  nextFrame = 0;
});

function frame(eventId: string, event: RunEvent["event"], segmentId = SEGMENT): RunEvent {
  return {
    runId: RUN,
    segmentId,
    eventId,
    timestamp: "2026-07-29T00:00:00Z",
    event,
  } as RunEvent;
}

const progressed: RunEvent["event"] = { type: "segment.progress", progress: { step: 1 } } as never;
const finished: RunEvent["event"] = {
  type: "segment.finished",
  outcome: { type: "completed" },
  metrics: { steps: 1, activeDurationMillis: 1 },
} as never;

function streamOf(events: RunEvent[], headEventId?: string): RunStream {
  return {
    result: { runId: RUN, segmentId: SEGMENT, ...(headEventId ? { headEventId } : {}) },
    events: (async function* () {
      for (const ev of events) yield ev;
    })(),
  };
}

function terminalRun(): RunRef {
  return {
    id: RUN,
    sessionId: asSessionId("ses_1"),
    status: "finished",
    createdAt: "2026-07-29T00:00:00Z",
    finishedAt: "2026-07-29T00:00:01Z",
    outcome: { type: "completed" },
    metrics: { steps: 1, activeDurationMillis: 1 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  };
}

function pumpWith(reattach: (position: RunStreamPosition) => Promise<RunStream | null>) {
  const positions: RunStreamPosition[] = [];
  const pump = createAgentRunPump({
    sessionId: "ses_1",
    isCancelled: () => false,
    readEpoch: () => 0n,
    applyEvents: vi.fn(() => true),
    reattach: (position) => {
      positions.push(position);
      return reattach(position);
    },
  });
  return { pump, positions };
}

describe("agent run pump reattach", () => {
  it("resumes from the last event it folded when a stream ends without a terminal", async () => {
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_9", finished)])),
    );

    await pump.pump(streamOf([frame("evt_7", progressed)]), new AbortController().signal);

    expect(positions).toHaveLength(1);
    expect(positions[0]).toMatchObject({
      runId: RUN,
      segmentId: SEGMENT,
      lastEventId: "evt_7",
      recovery: "replay",
    });
  });

  it("does not advance the replay cursor past a batch rejected by a newer view epoch", async () => {
    let epoch = 0n;
    const positions: RunStreamPosition[] = [];
    const applyEvents = vi.fn();
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => epoch,
      applyEvents,
      reattach: (position) => {
        positions.push(position);
        return Promise.resolve(null);
      },
    });
    const interrupted: RunStream = {
      result: { runId: RUN, segmentId: SEGMENT },
      events: (async function* () {
        yield frame("evt_not_folded", progressed);
        epoch = 1n;
      })(),
    };

    await pump.pump(interrupted, new AbortController().signal);

    expect(applyEvents).not.toHaveBeenCalled();
    expect(positions[0]?.lastEventId).toBe("");
  });

  it("requests cold recovery after an authoritative protocol violation", async () => {
    const failed: RunStream = {
      result: { runId: RUN, segmentId: SEGMENT },
      events: (async function* () {
        yield frame("evt_7", progressed);
        throw new RpcProtocolError("notifications.run.event params", [
          { path: "event.item", detail: "is required" },
        ]);
      })(),
    };
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_9", finished)])),
    );

    await pump.pump(failed, new AbortController().signal);

    expect(positions[0]).toMatchObject({ lastEventId: "evt_7", recovery: "cold" });
  });

  it("does not diagnose an expected Runtime connection loss as an Agent failure", async () => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const failed: RunStream = {
      result: { runId: RUN, segmentId: SEGMENT },
      events: (async function* () {
        yield frame("evt_before_disconnect", progressed);
        throw new RpcConnectionError("network error");
      })(),
    };
    const readRunSnapshot = vi.fn(() => Promise.reject(new RpcConnectionError("fetch failed")));
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
      readRunSnapshot,
      reattach: () => Promise.resolve(null),
    });

    await pump.pump(failed, new AbortController().signal);

    expect(readRunSnapshot).toHaveBeenCalledOnce();
    expect(warning).not.toHaveBeenCalled();
  });

  it("hands back the head the attach captured when it folded nothing", async () => {
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_5", finished)])),
    );

    await pump.pump(streamOf([], "evt_head"), new AbortController().signal);

    expect(positions[0]?.lastEventId).toBe("evt_head");
  });

  it("stops at the segment's own terminal", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));

    await pump.pump(
      streamOf([frame("evt_1", progressed), frame("evt_2", finished)]),
      new AbortController().signal,
    );

    expect(positions).toHaveLength(0);
  });

  it("folds the stream tail before publishing the idle ownership boundary", async () => {
    const order: string[] = [];
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: (events) => {
        order.push(...events.map((entry) => entry.event.type));
        return true;
      },
      readRunSnapshot: async () => {
        order.push("exact-read");
        return terminalRun();
      },
      applyRunSnapshot: () => order.push("snapshot"),
      onIdle: () => order.push("idle"),
    });

    await pump.pump(
      streamOf([frame("evt_1", progressed), frame("evt_2", finished)]),
      new AbortController().signal,
    );

    expect(order).toEqual([
      "segment.progress",
      "segment.finished",
      "exact-read",
      "snapshot",
      "idle",
    ]);
    expect(pump.isActive()).toBe(false);
  });

  it("does not fold an older exact read after a newer pump takes ownership", async () => {
    const exactRead = deferred<RunRef>();
    const applyRunSnapshot = vi.fn();
    const readRunSnapshot = vi.fn(() => exactRead.promise);
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
      readRunSnapshot,
      applyRunSnapshot,
    });
    const oldController = new AbortController();
    const newController = new AbortController();
    const newerSegment = asSegmentId("seg_newer");

    const older = pump.pump(streamOf([frame("evt_old_terminal", finished)]), oldController.signal);
    await vi.waitFor(() => expect(readRunSnapshot).toHaveBeenCalledOnce());
    const newer = pump.pump(parkedStream(newerSegment, newController.signal), newController.signal);
    await Promise.resolve();

    exactRead.resolve(terminalRun());
    await older;
    expect(applyRunSnapshot).not.toHaveBeenCalled();
    expect(pump.isFollowing(RUN, newerSegment)).toBe(true);

    newController.abort();
    await newer;
  });

  it("releases live ownership and ignores a late exact read after abort", async () => {
    const exactRead = deferred<RunRef>();
    const applyRunSnapshot = vi.fn();
    const onIdle = vi.fn();
    const readRunSnapshot = vi.fn(() => exactRead.promise);
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
      readRunSnapshot,
      applyRunSnapshot,
      onIdle,
    });
    const controller = new AbortController();
    const running = pump.pump(
      streamOf([frame("evt_terminal_before_abort", finished)]),
      controller.signal,
    );
    await vi.waitFor(() => expect(readRunSnapshot).toHaveBeenCalledOnce());

    controller.abort();
    await running;

    expect(pump.isActive()).toBe(false);
    expect(onIdle).toHaveBeenCalledOnce();
    expect(applyRunSnapshot).not.toHaveBeenCalled();

    exactRead.resolve(terminalRun());
    await Promise.resolve();
    expect(applyRunSnapshot).not.toHaveBeenCalled();
  });

  it("keeps its own cursor across a replaying reattach", async () => {
    // The ack of a reattach reports the head as of that attach, which is AHEAD of the
    // cursor being replayed from. Adopting it would skip the replay.
    let attempt = 0;
    const { pump, positions } = pumpWith(() => {
      attempt += 1;
      return Promise.resolve(
        attempt === 1
          ? streamOf([], "evt_head_ahead")
          : streamOf([frame("evt_terminal", finished)]),
      );
    });

    await pump.pump(streamOf([frame("evt_3", progressed)]), new AbortController().signal);

    expect(positions.map((p) => p.lastEventId)).toEqual(["evt_3", "evt_3"]);
  });

  it("gives up when the run is no longer attachable", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));

    await pump.pump(streamOf([frame("evt_4", progressed)]), new AbortController().signal);

    expect(positions).toHaveLength(1);
  });

  it("does not reattach a stream the caller aborted", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));
    const ctrl = new AbortController();
    ctrl.abort();

    await pump.pump(streamOf([frame("evt_6", progressed)]), ctrl.signal);

    expect(positions).toHaveLength(0);
  });

  it("releases live ownership when an iterator ignores cancellation", async () => {
    let releaseNext!: (result: IteratorResult<RunEvent>) => void;
    const close = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const onIdle = vi.fn();
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
      onIdle,
    });
    const controller = new AbortController();
    const running = pump.pump(
      {
        result: { runId: RUN, segmentId: SEGMENT },
        events: {
          [Symbol.asyncIterator]: () => ({
            next: () =>
              new Promise<IteratorResult<RunEvent>>((resolve) => {
                releaseNext = resolve;
              }),
            return: close,
          }),
        },
      },
      controller.signal,
    );
    await vi.waitFor(() => expect(releaseNext).toBeTypeOf("function"));

    controller.abort();
    await running;

    expect(close).toHaveBeenCalledOnce();
    expect(onIdle).toHaveBeenCalledOnce();
    expect(pump.isActive()).toBe(false);
    releaseNext({ value: undefined as never, done: true });
  });

  it("does not let a retired pump reattach after its iterator settles late", async () => {
    const lateNext = deferred<IteratorResult<RunEvent>>();
    const reattach = vi.fn(() => Promise.resolve(null));
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
      reattach,
    });
    const oldController = new AbortController();
    const newController = new AbortController();
    const oldSegment = asSegmentId("seg_old");
    const newSegment = asSegmentId("seg_new");
    const older = pump.pump(
      {
        result: { runId: RUN, segmentId: oldSegment },
        events: {
          [Symbol.asyncIterator]: () => ({ next: () => lateNext.promise }),
        },
      },
      oldController.signal,
    );
    await Promise.resolve();

    const newer = pump.pump(parkedStream(newSegment, newController.signal), newController.signal);
    await Promise.resolve();
    lateNext.resolve({ value: undefined as never, done: true });
    await older;

    expect(reattach).not.toHaveBeenCalled();
    newController.abort();
    await newer;
  });

  it("does not let an older pump clear a newer segment for the same run", async () => {
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0n,
      applyEvents: vi.fn(() => true),
    });
    const oldController = new AbortController();
    const newController = new AbortController();
    const oldSegment = asSegmentId("seg_old");
    const newSegment = asSegmentId("seg_new");

    const older = pump.pump(parkedStream(oldSegment, oldController.signal), oldController.signal);
    await Promise.resolve();
    const newer = pump.pump(parkedStream(newSegment, newController.signal), newController.signal);
    await Promise.resolve();
    expect(pump.isFollowing(RUN, newSegment)).toBe(true);

    oldController.abort();
    await older;
    expect(pump.isFollowing(RUN, newSegment)).toBe(true);

    newController.abort();
    await newer;
    expect(pump.isFollowing(RUN, newSegment)).toBe(false);
  });
});

function parkedStream(segmentId: ReturnType<typeof asSegmentId>, signal: AbortSignal): RunStream {
  return {
    result: { runId: RUN, segmentId },
    events: {
      [Symbol.asyncIterator]() {
        return {
          async next(): Promise<IteratorResult<RunEvent>> {
            await new Promise<never>((_, reject) => {
              if (signal.aborted) {
                reject(new Error("aborted"));
                return;
              }
              signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
            });
            return { done: true, value: undefined as never };
          },
        };
      },
    },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
