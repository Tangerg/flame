import {
  ASYNC_OWNERSHIP_RETIRED as ABORTED,
  disposeAsyncIterator,
  settleBeforeAbort,
  settleWithinNextTask,
} from "@/lib/asyncOwnership";
import { delayUntilAborted } from "@/lib/abortableDelay";
import {
  RpcConnectionError,
  RpcProtocolError,
  type RunEvent,
  type RunId,
  type RunRef,
  type SegmentId,
  type StreamingResult,
} from "@flame/runtime-client";
import { createRunEventBatcher } from "@/features/agent/adapters/runEventBatcher";
interface RunStreamAck {
  runId: RunId;
  segmentId: SegmentId;
  headEventId?: string;
}

export type RunStream = StreamingResult<RunStreamAck, RunEvent>;

export interface RunStreamReattachment extends RunStream {
  cursor: string | null;
}

export interface RunStreamPosition {
  runId: RunId;
  segmentId: SegmentId;
  lastEventId: string | null;
  recovery: "replay" | "cold";
}

interface DrainedStream {
  finished: boolean;
  recovery: RunStreamPosition["recovery"];
  error?: unknown;
}

const MAXIMUM_STALLED_REPLAYS = 3;
const REATTACH_BACKOFF_BASE_MILLISECONDS = 50;
const REATTACH_BACKOFF_CAP_MILLISECONDS = 1000;

// Replays that leave the position unchanged escalate to one cold recovery, and a
// cold recovery that also makes no progress fails rather than retrying forever.
class ReattachPolicy {
  #lastDetached: RunStreamPosition;
  #stalledReplays = 0;
  #coldRecovered = false;

  constructor(position: RunStreamPosition) {
    this.#lastDetached = position;
  }

  next(position: RunStreamPosition, projectionAdvanced: boolean, drained: DrainedStream) {
    const segmentChanged = position.segmentId !== this.#lastDetached.segmentId;
    const advanced = segmentChanged || position.lastEventId !== this.#lastDetached.lastEventId;
    this.#stalledReplays = advanced ? 0 : this.#stalledReplays + 1;
    if ((advanced && projectionAdvanced) || segmentChanged) this.#coldRecovered = false;
    this.#lastDetached = position;
    if (drained.recovery !== "cold" && this.#stalledReplays < MAXIMUM_STALLED_REPLAYS)
      return position;
    if (this.#coldRecovered)
      throw new Error("run synchronization incomplete: recovery made no progress", {
        cause: drained.error,
      });
    this.#coldRecovered = true;
    return { ...position, recovery: "cold" as const };
  }

  backoffMilliseconds(): number {
    return Math.min(
      REATTACH_BACKOFF_BASE_MILLISECONDS * 2 ** this.#stalledReplays,
      REATTACH_BACKOFF_CAP_MILLISECONDS,
    );
  }
}

interface AgentRunPumpOptions {
  sessionId: string;
  onRunFinished: () => void;
  isCancelled: () => boolean;
  readEpoch: () => bigint;
  applyEvents: (events: RunEvent[]) => boolean;
  readRunSnapshot?: (runId: RunId, signal: AbortSignal) => Promise<RunRef>;
  applyRunSnapshot?: (run: RunRef) => void;
  reattach?: (
    position: RunStreamPosition,
    signal: AbortSignal,
  ) => Promise<RunStreamReattachment | null>;
  onSynchronizationFailed?: (error: unknown) => void;
  onIdle?: () => void;
}

interface AgentRunPump {
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
  isFollowing: (runId: string, segmentId: string) => boolean;
  isActive: () => boolean;
  dispose: () => void;
}

export function createAgentRunPump({
  sessionId,
  onRunFinished,
  isCancelled,
  readEpoch,
  applyEvents,
  readRunSnapshot,
  applyRunSnapshot,
  reattach,
  onIdle,
  onSynchronizationFailed,
}: AgentRunPumpOptions): AgentRunPump {
  let currentRunId: RunId | null = null;
  let currentSegmentId: SegmentId | null = null;
  let currentPumpLease: object = {};
  let activeBatcher: ReturnType<typeof createRunEventBatcher> | null = null;

  return {
    async pump(stream, signal) {
      const pumpLease = (currentPumpLease = {});
      const superseded = () => currentPumpLease !== pumpLease || isCancelled() || signal.aborted;
      const runId = stream.result.runId;
      currentRunId = runId;
      currentSegmentId = stream.result.segmentId;
      let position: RunStreamPosition = {
        runId,
        segmentId: stream.result.segmentId,
        lastEventId: stream.result.headEventId ?? null,
        recovery: "replay",
      };
      activeBatcher?.dispose();
      let eventBatcher: ReturnType<typeof createRunEventBatcher> | null = null;
      const reattachPolicy = new ReattachPolicy(position);
      let events: AsyncIterable<RunEvent> | null = stream.events;
      try {
        while (events) {
          const projectionFailure = new AbortController();
          let projectionAdvanced = false;
          eventBatcher = createRunEventBatcher({
            readEpoch,
            apply: applyEvents,
            onFailure: (error) => projectionFailure.abort(error),
            onApplied: (event) => {
              projectionAdvanced ||= event.eventId !== position.lastEventId;
              position = { ...position, lastEventId: event.eventId };
            },
            onRunFinished,
          });
          activeBatcher = eventBatcher;
          const drained = await consume(
            events,
            position.segmentId,
            AbortSignal.any([signal, projectionFailure.signal]),
            eventBatcher,
          );
          eventBatcher.flush();
          eventBatcher.dispose();
          if (projectionFailure.signal.aborted) {
            if (!reattach) {
              throw new Error("run synchronization incomplete", {
                cause: projectionFailure.signal.reason,
              });
            }
            drained.finished = false;
            drained.recovery = "cold";
            drained.error = projectionFailure.signal.reason;
          }
          if (superseded() || drained.finished || !reattach) break;
          position = reattachPolicy.next(position, projectionAdvanced, drained);
          await delayUntilAborted(reattachPolicy.backoffMilliseconds(), signal);
          if (superseded()) break;
          const next = await reattach(position, signal);
          if (!next) break;
          if (superseded()) {
            await disposeAsyncIterator(next.events[Symbol.asyncIterator]());
            break;
          }
          position = {
            runId,
            segmentId: next.result.segmentId,
            lastEventId: next.cursor,
            recovery: "replay",
          };
          currentSegmentId = next.result.segmentId;
          events = next.events;
        }
      } catch (error) {
        if (!superseded()) {
          console.warn("[agent] run synchronization failed:", { sessionId, ...position }, error);
          onSynchronizationFailed?.(error);
        }
        throw error;
      } finally {
        eventBatcher?.dispose();
        if (activeBatcher === eventBatcher) activeBatcher = null;
        await settleIdle(pumpLease, runId, signal);
      }
    },
    isFollowing(runId, segmentId) {
      return currentRunId === runId && currentSegmentId === segmentId;
    },
    isActive() {
      return currentRunId !== null;
    },
    dispose() {
      activeBatcher?.dispose();
      activeBatcher = null;
    },
  };

  async function settleIdle(pumpLease: object, runId: RunId, signal: AbortSignal) {
    if (currentPumpLease !== pumpLease) return;
    const snapshot = await readFinalSnapshot(runId, signal);
    if (currentPumpLease !== pumpLease) return;
    if (snapshot && !isCancelled() && !signal.aborted) applyRunSnapshot?.(snapshot);
    currentRunId = null;
    currentSegmentId = null;
    onIdle?.();
  }

  async function readFinalSnapshot(runId: RunId, signal: AbortSignal) {
    if (!readRunSnapshot || isCancelled() || signal.aborted) return undefined;
    try {
      const read = await settleBeforeAbort(readRunSnapshot(runId, signal), signal);
      return read === ABORTED ? undefined : read;
    } catch (error) {
      if (!isCancelled() && !signal.aborted && !(error instanceof RpcConnectionError))
        console.warn("[agent] exact run read failed:", sessionId, runId, error);
      return undefined;
    }
  }

  async function consume(
    events: AsyncIterable<RunEvent>,
    rootSegmentId: SegmentId,
    signal: AbortSignal,
    eventBatcher: ReturnType<typeof createRunEventBatcher>,
  ): Promise<DrainedStream> {
    let finished = false;
    let recovery: "replay" | "cold" = "replay";
    let error: unknown;
    const iterator = events[Symbol.asyncIterator]();
    let iteratorDone = false;
    try {
      while (!signal.aborted && !isCancelled()) {
        const pendingNext = Promise.resolve(iterator.next());
        const next = await settleBeforeAbort(pendingNext, signal);
        if (next === ABORTED) {
          const lateNext = await settleWithinNextTask(pendingNext);
          if (lateNext.status === "fulfilled" && lateNext.value.done) iteratorDone = true;
          return { finished: true, recovery };
        }
        if (next.done) {
          iteratorDone = true;
          break;
        }
        if (isCancelled() || signal.aborted) return { finished: true, recovery };
        const ev = next.value;
        eventBatcher.enqueue(ev);
        if (ev.segmentId === rootSegmentId && ev.event.type === "segment.finished") {
          finished = true;
        }
      }
    } catch (err) {
      error = err;
      if (err instanceof RpcProtocolError) recovery = "cold";
      if (!isCancelled() && !signal.aborted && !(err instanceof RpcConnectionError))
        console.warn("[agent] run stream ended early:", sessionId, err);
    } finally {
      if (!iteratorDone) await disposeAsyncIterator(iterator);
    }
    return { finished, recovery, error };
  }
}
