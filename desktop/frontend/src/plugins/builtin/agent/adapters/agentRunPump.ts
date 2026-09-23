import {
  ASYNC_OWNERSHIP_RETIRED as ABORTED,
  disposeAsyncIterator,
  settleBeforeAbort,
  settleWithinNextTask,
} from "@/lib/asyncOwnership";
import { queryClient } from "@/lib/queryClient";
import {
  RpcConnectionError,
  RpcProtocolError,
  type RunEvent,
  type RunId,
  type RunRef,
  type SegmentId,
  type StreamingResult,
} from "@/rpc";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import { createRunEventBatcher } from "./runEventBatcher";

interface RunStreamAck {
  runId: RunId;
  segmentId: SegmentId;
  headEventId?: string;
}

export type RunStream = StreamingResult<RunStreamAck, RunEvent>;

export interface RunStreamReattachment extends RunStream {
  cursor: string;
}

export interface RunStreamPosition {
  runId: RunId;
  segmentId: SegmentId;
  lastEventId: string;
  recovery: "replay" | "cold";
}

interface AgentRunPumpOptions {
  sessionId: string;
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
      const runId = stream.result.runId;
      currentRunId = runId;
      currentSegmentId = stream.result.segmentId;
      let position: RunStreamPosition = {
        runId,
        segmentId: stream.result.segmentId,
        lastEventId: stream.result.headEventId ?? "",
        recovery: "replay",
      };
      activeBatcher?.dispose();
      let eventBatcher: ReturnType<typeof createRunEventBatcher> | null = null;
      let projectionRecovered = false;
      let events: AsyncIterable<RunEvent> | null = stream.events;
      try {
        while (events) {
          const projectionFailure = new AbortController();
          eventBatcher = createRunEventBatcher({
            readEpoch,
            apply: applyEvents,
            onFailure: (error) => projectionFailure.abort(error),
            onApplied: (event) => {
              position = { ...position, lastEventId: event.eventId };
            },
            onRunFinished: () => {
              void queryClient.invalidateQueries({
                queryKey: [AGENT_SESSION_USAGE_KEY, sessionId],
              });
            },
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
            if (projectionRecovered || !reattach) {
              throw new Error("run synchronization incomplete", {
                cause: projectionFailure.signal.reason,
              });
            }
            projectionRecovered = true;
            drained.finished = false;
            drained.recovery = "cold";
          }
          if (drained.recovery === "cold") position = { ...position, recovery: "cold" };
          if (
            currentPumpLease !== pumpLease ||
            drained.finished ||
            !reattach ||
            isCancelled() ||
            signal.aborted
          )
            break;
          const next = await reattach(position, signal);
          if (!next) break;
          if (currentPumpLease !== pumpLease || isCancelled() || signal.aborted) {
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
        if (currentPumpLease === pumpLease && !isCancelled() && !signal.aborted)
          onSynchronizationFailed?.(error);
        throw error;
      } finally {
        eventBatcher?.dispose();
        if (activeBatcher === eventBatcher) activeBatcher = null;
        if (currentPumpLease === pumpLease) {
          let snapshot: RunRef | undefined;
          if (readRunSnapshot && !isCancelled() && !signal.aborted) {
            try {
              const read = await settleBeforeAbort(readRunSnapshot(runId, signal), signal);
              if (read !== ABORTED) snapshot = read;
            } catch (error) {
              if (!isCancelled() && !signal.aborted && !(error instanceof RpcConnectionError)) {
                console.warn("[agent] exact run read failed:", sessionId, runId, error);
              }
            }
          }

          if (currentPumpLease === pumpLease) {
            if (snapshot && !isCancelled() && !signal.aborted) applyRunSnapshot?.(snapshot);
            currentRunId = null;
            currentSegmentId = null;
            onIdle?.();
          }
        }
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

  async function consume(
    events: AsyncIterable<RunEvent>,
    rootSegmentId: SegmentId,
    signal: AbortSignal,
    eventBatcher: ReturnType<typeof createRunEventBatcher>,
  ): Promise<{ finished: boolean; recovery: "replay" | "cold" }> {
    let finished = false;
    let recovery: "replay" | "cold" = "replay";
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
      if (err instanceof RpcProtocolError) recovery = "cold";
      if (!isCancelled() && !signal.aborted && !(err instanceof RpcConnectionError))
        console.warn("[agent] run stream ended early:", sessionId, err);
    } finally {
      if (!iteratorDone) await disposeAsyncIterator(iterator);
    }
    return { finished, recovery };
  }
}
