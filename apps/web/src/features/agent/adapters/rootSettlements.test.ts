import { beforeEach, onTestFinished, describe, expect, it, vi } from "vitest";
import {
  asRunId,
  asSegmentId,
  type RunEvent,
  type RunOutcome,
  type RunRef,
} from "@flame/runtime-client";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { createRunEventBatcher } from "./runEventBatcher";
import { createAgentRunPump } from "./agentRunPump";

let testClient: TestClient;
const sessionId = "session-settlements";
const runId = "run-settlements";
const segmentId = "segment-settlements";
const run: RunRef = {
  id: runId,
  sessionId,
  status: "running",
  activeSegmentId: segmentId,
  createdAt: "2026-06-03T00:00:00.000Z",
  metrics: { steps: 0, activeDurationMillis: 0 },
  protocolProfile: { interruptTypes: ["question"], requiredFeatures: [] },
  provider: "openai",
  model: "gpt-5",
};
function envelope(
  event: RunEvent["event"],
  id: string,
  sourceRun = runId,
  sourceSegment = segmentId,
): RunEvent {
  return {
    event,
    eventId: id,
    runId: sourceRun,
    segmentId: sourceSegment,
    timestamp: "2026-06-03T00:00:01.000Z",
  };
}
const started = envelope({ type: "segment.started", run }, "started");
function ended(outcome: RunOutcome = { type: "completed" }): RunEvent {
  const { activeSegmentId: _segment, ...rest } = run;
  return envelope(
    {
      type: "segment.finished",
      run: { ...rest, status: "finished", outcome, finishedAt: "2026-06-03T00:00:01.000Z" },
    },
    "finished",
  );
}
function waiting(): RunEvent {
  const { activeSegmentId: _segment, ...rest } = run;
  return envelope(
    {
      type: "segment.finished",
      run: { ...rest, status: "waiting" },
      interrupts: [
        {
          type: "question",
          itemId: "question-1",
          runId,
          payload: { question: { fields: [{ type: "text", prompt: "Continue?" }] } },
        },
      ],
    },
    "waiting",
  );
}

beforeEach(() => {
  testClient = createTestClient();
  onTestFinished(() => testClient.dispose());
});

function pipeline() {
  const owner = testClient.agent.projection;
  const store = owner.store;
  store.getState().ensureSession(sessionId);
  const onSettled = vi.fn();
  onTestFinished(testClient.agent.access.subscribeRootRunSettlements(onSettled));
  const scheduled = new Map<number, () => void>();
  let nextFrame = 0;
  const onFailure = vi.fn();
  const batcher = createRunEventBatcher({
    readEpoch: () => store.getState().sessions[sessionId]?.viewEpoch ?? 0n,
    apply: (events) => store.getState().applyRunEvents(sessionId, events),
    onFailure,
    scheduleFrame: (flush) => {
      const id = ++nextFrame;
      scheduled.set(id, flush);
      return id;
    },
    cancelFrame: (id) => {
      scheduled.delete(id);
    },
  });
  onTestFinished(() => batcher.dispose());
  const frame = () => {
    const pending = [...scheduled.values()];
    scheduled.clear();
    for (const flush of pending) flush();
  };
  return { store, batcher, frame, onSettled, onFailure };
}

const outcomeCases: Array<{ outcome: RunOutcome; status: string; errorMessage: string | null }> = [
  { outcome: { type: "completed" }, status: "finished", errorMessage: null },
  { outcome: { type: "canceled" }, status: "canceled", errorMessage: null },
  {
    outcome: { type: "failed", error: { type: "provider_error", detail: "provider refused" } },
    status: "error",
    errorMessage: "provider refused",
  },
  {
    outcome: { type: "timedOut", error: { type: "provider_error", detail: "deadline reached" } },
    status: "error",
    errorMessage: "deadline reached",
  },
  {
    outcome: { type: "lost", error: { type: "provider_error", detail: "runtime restarted" } },
    status: "error",
    errorMessage: "runtime restarted",
  },
];

describe("committed root Run settlements", () => {
  it.each(outcomeCases)(
    "publishes $outcome.type once when start and end share one frame",
    ({ outcome, status, errorMessage }) => {
      const { store, batcher, frame, onSettled } = pipeline();
      const onProjection = vi.fn();
      onTestFinished(store.subscribe(onProjection));
      const finish = ended(outcome);
      batcher.enqueue(started);
      batcher.enqueue(finish);
      frame();
      expect(onProjection).toHaveBeenCalledOnce();
      expect(onSettled).toHaveBeenCalledExactlyOnceWith({
        sessionId,
        runId,
        segmentId,
        status,
        errorMessage,
      });
      batcher.enqueue(finish);
      frame();
      expect(onSettled).toHaveBeenCalledOnce();
    },
  );

  it("publishes once across separate frames", () => {
    const { batcher, frame, onSettled } = pipeline();
    batcher.enqueue(started);
    frame();
    expect(onSettled).not.toHaveBeenCalled();
    batcher.enqueue(ended());
    frame();
    expect(onSettled).toHaveBeenCalledOnce();
  });

  it("publishes each admitted waiting and terminal segment without replaying the waiting event", () => {
    const { batcher, frame, onSettled } = pipeline();
    const parked = waiting();
    batcher.enqueue(started);
    batcher.enqueue(parked);
    frame();
    expect(onSettled).toHaveBeenCalledExactlyOnceWith({
      sessionId,
      runId,
      segmentId,
      status: "needsInput",
      errorMessage: null,
    });
    batcher.enqueue(parked);
    frame();
    expect(onSettled).toHaveBeenCalledOnce();
    const nextSegment = "segment-resumed";
    batcher.enqueue(
      envelope(
        { type: "segment.started", run: { ...run, activeSegmentId: nextSegment } },
        "resumed",
        runId,
        nextSegment,
      ),
    );
    batcher.enqueue({ ...ended(), eventId: "resumed-finish", segmentId: nextSegment });
    frame();
    expect(onSettled).toHaveBeenCalledTimes(2);
    expect(onSettled).toHaveBeenLastCalledWith({
      sessionId,
      runId,
      segmentId: nextSegment,
      status: "finished",
      errorMessage: null,
    });
  });

  it("flushes the hidden-window tail through the real Run pump", async () => {
    const { store, onSettled } = pipeline();
    vi.stubGlobal("requestAnimationFrame", () => 1);
    vi.stubGlobal("cancelAnimationFrame", () => undefined);
    onTestFinished(() => {
      vi.unstubAllGlobals();
    });
    const pump = createAgentRunPump({
      sessionId,
      isCancelled: () => false,
      onRunFinished: vi.fn(),
      readEpoch: () => store.getState().sessions[sessionId]!.viewEpoch,
      applyEvents: (events) => store.getState().applyRunEvents(sessionId, events),
    });
    onTestFinished(() => pump.dispose());
    await pump.pump(
      {
        result: { runId: asRunId(runId), segmentId: asSegmentId(segmentId) },
        events: (async function* () {
          yield started;
          yield ended();
        })(),
      },
      new AbortController().signal,
    );
    expect(onSettled).toHaveBeenCalledOnce();
  });

  it("does not publish a child's settlement", () => {
    const { batcher, frame, onSettled } = pipeline();
    const child: RunRef = {
      ...run,
      id: "child",
      parentRunId: runId,
      rootRunId: runId,
      spawnedByItemId: "spawn",
      activeSegmentId: "child-segment",
    };
    batcher.enqueue(started);
    batcher.enqueue(
      envelope({ type: "segment.started", run: child }, "child-started", child.id, "child-segment"),
    );
    const { activeSegmentId: _segment, ...rest } = child;
    batcher.enqueue(
      envelope(
        {
          type: "segment.finished",
          run: {
            ...rest,
            status: "finished",
            outcome: { type: "completed" },
            finishedAt: "2026-06-03T00:00:01.000Z",
          },
        },
        "child-finished",
        child.id,
        "child-segment",
      ),
    );
    frame();
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("keeps initial history and cold snapshots quiet", () => {
    const { store, batcher, frame, onSettled } = pipeline();
    store.getState().applyRunSnapshot(sessionId, run);
    const event = ended();
    if (event.event.type !== "segment.finished")
      throw new Error("expected a terminal event fixture");
    store.getState().applyRunSnapshot(sessionId, event.event.run);
    const token = store.getState().beginViewRefresh(sessionId, true)!;
    const view = store.getState().sessions[sessionId]!.view;
    expect(store.getState().commitViewRefresh(sessionId, token, view)).toBe(true);
    batcher.enqueue(event);
    frame();
    expect(onSettled).not.toHaveBeenCalled();
  });

  it.each([false, true])("projects an accepted cancellation receipt once (parked=$0)", (parked) => {
    const { store, batcher, frame, onSettled } = pipeline();
    batcher.enqueue(started);
    if (parked) batcher.enqueue(waiting());
    frame();
    onSettled.mockClear();
    const expected = store.getState().sessions[sessionId]!;
    const finish = ended({ type: "canceled" });
    if (finish.event.type !== "segment.finished") throw new Error("expected terminal fixture");
    expect(
      store
        .getState()
        .commitCancelResponse(sessionId, expected, { type: "root", run: finish.event.run }),
    ).toBe(true);
    expect(onSettled).toHaveBeenCalledExactlyOnceWith({
      sessionId,
      runId,
      segmentId: parked ? null : segmentId,
      status: "canceled",
      errorMessage: null,
    });
    batcher.enqueue(finish);
    frame();
    expect(onSettled).toHaveBeenCalledOnce();
  });

  it("does not publish a refused cancellation receipt", () => {
    const { store, batcher, frame, onSettled } = pipeline();
    const stale = store.getState().sessions[sessionId]!;
    batcher.enqueue(started);
    frame();
    const finish = ended({ type: "canceled" });
    if (finish.event.type !== "segment.finished") throw new Error("expected terminal fixture");
    expect(
      store
        .getState()
        .commitCancelResponse(sessionId, stale, { type: "root", run: finish.event.run }),
    ).toBe(false);
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("does not publish any transition from a refused atomic batch", () => {
    const { store, batcher, frame, onSettled, onFailure } = pipeline();
    batcher.enqueue(started);
    batcher.enqueue(ended());
    batcher.enqueue(started);
    frame();
    expect(onFailure).toHaveBeenCalledOnce();
    expect(onSettled).not.toHaveBeenCalled();
    expect(store.getState().sessions[sessionId]!.view.runsById[runId]).toBeUndefined();
  });

  it("drops a queued batch when its projection generation retires", () => {
    const { store, batcher, frame, onSettled } = pipeline();
    batcher.enqueue(started);
    batcher.enqueue(ended());
    store.getState().retireProjectionGeneration([sessionId]);
    frame();
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("retires settlement subscribers with the projection owner", () => {
    const { batcher, frame, onSettled } = pipeline();
    testClient.agent.projection.dispose();
    batcher.enqueue(started);
    batcher.enqueue(ended());
    frame();
    expect(onSettled).not.toHaveBeenCalled();
  });

  it("does not publish after a subscriber retires the committed generation", () => {
    const { store, batcher, frame, onSettled } = pipeline();
    const stop = store.subscribe(() => {
      stop();
      store.getState().retireProjectionGeneration([sessionId]);
    });
    onTestFinished(stop);
    batcher.enqueue(started);
    batcher.enqueue(ended());
    frame();
    expect(onSettled).not.toHaveBeenCalled();
  });
});
