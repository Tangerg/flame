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

/** headEventId exists only on a REATTACH: a start or resume stream begins at the beginning
 *  of its segment, so there is no earlier position to name. */
interface RunStreamAck {
  runId: RunId;
  segmentId: SegmentId;
  headEventId?: string;
}

export type RunStream = StreamingResult<RunStreamAck, RunEvent>;

/** lastEventId is empty when this client folded nothing and was given no head; the reattach
 *  is then tail-only and the durable snapshot supplies the projection. */
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
  /** Stream terminal events are compact; runs.get is the authoritative RunRef. */
  readRunSnapshot?: (runId: RunId, signal: AbortSignal) => Promise<RunRef>;
  applyRunSnapshot?: (run: RunRef) => void;
  /** null means no longer attachable at all — finished, waiting on a person, or moved to
   *  another segment — after the durable projection reconciled that transition. */
  reattach?: (position: RunStreamPosition, signal: AbortSignal) => Promise<RunStream | null>;
  /** The newest live stream became idle after its queued tail was folded. */
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
}: AgentRunPumpOptions): AgentRunPump {
  let currentRunId: RunId | null = null;
  let currentSegmentId: SegmentId | null = null;
  let currentPumpLease: object = {};
  let activeBatcher: ReturnType<typeof createRunEventBatcher> | null = null;

  return {
    // A run OUTLIVES its stream: ending without the segment's terminal is an abnormal EOS,
    // and the run keeps executing on the server. Reattaching from the last folded event
    // turns that into a gap of milliseconds rather than a transcript frozen until reload.
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
      const eventBatcher = createRunEventBatcher({
        readEpoch,
        apply: applyEvents,
        onApplied: (event) => {
          position = { ...position, lastEventId: event.eventId };
        },
        onRunFinished: () => {
          void queryClient.invalidateQueries({ queryKey: [AGENT_SESSION_USAGE_KEY, sessionId] });
        },
      });
      activeBatcher = eventBatcher;
      let events: AsyncIterable<RunEvent> | null = stream.events;
      try {
        while (events) {
          const drained = await consume(events, position.segmentId, signal, eventBatcher);
          eventBatcher.flush();
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
            // Only adopt the ack's head with no cursor of our own: a replaying attach's
            // head sits AHEAD of what was asked for, so taking it skips the replay.
            lastEventId: position.lastEventId || (next.result.headEventId ?? ""),
            recovery: "replay",
          };
          currentSegmentId = next.result.segmentId;
          events = next.events;
        }
      } finally {
        eventBatcher.flush();
        if (activeBatcher === eventBatcher) activeBatcher = null;
        if (currentPumpLease === pumpLease) {
          // The durable change stream may already have requested a projection
          // refresh. Land this stream's rAF-delayed tail before declaring the
          // session idle, otherwise the newer snapshot can overtake it.
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

          // A newer pump may have opened while the exact read was in flight.
          // Its stream owns the projection now, so the older RunRef cannot be
          // folded and the older finally cannot publish an idle boundary.
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
        // An aborted request or a torn-down session is a deliberate stop, not a gap
        // to recover: nothing is reattached after it.
        if (isCancelled() || signal.aborted) return { finished: true, recovery };
        const ev = next.value;
        eventBatcher.enqueue(ev);
        // A descendant subagent's terminal rides this same stream; only the root
        // segment's ends it.
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
