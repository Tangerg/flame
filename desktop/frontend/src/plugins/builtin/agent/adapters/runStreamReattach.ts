import type { FlameClient } from "@/rpc";
import { asRunId, asSegmentId, RpcConnectionError } from "@/rpc";
import { agentRuntime } from "../application/ports/runtimeGateway";
import type { RunStreamReattachment, RunStreamPosition } from "./agentRunPump";
import { retireRunStream, settleRunStreamOpening } from "./runStreamOpening";
import { snapshotRunStream } from "./snapshotRunStream";

interface RunStreamReattachOptions {
  sessionId: string;
  client: () => Pick<FlameClient, "runs">;
  isCancelled: () => boolean;
  /** Refresh final durable material when the addressed Run can no longer be followed. */
  recoverProjection: (signal: AbortSignal) => Promise<void>;
}

/** Replay preserves its consumed cursor; cold recovery takes the coherent snapshot tail's head. */
export function createRunStreamReattach({
  sessionId,
  client,
  isCancelled,
  recoverProjection,
}: RunStreamReattachOptions) {
  return async function reattach(
    position: RunStreamPosition,
    signal: AbortSignal,
  ): Promise<RunStreamReattachment | null> {
    if (isCancelled() || signal.aborted) return null;
    const target = {
      runId: asRunId(position.runId),
      segmentId: asSegmentId(position.segmentId),
    };
    const recoverAndTail = async (): Promise<RunStreamReattachment | null> => {
      if (isCancelled() || signal.aborted) return null;
      try {
        const tail = await snapshotRunStream(
          client(),
          sessionId,
          target.runId,
          target.segmentId,
          signal,
          () => !isCancelled() && !signal.aborted,
        );
        if (!tail) return null;
        if (isCancelled() || signal.aborted) {
          retireRunStream(tail);
          return null;
        }
        return {
          result: brandAck(tail.result),
          events: tail.events,
          cursor: tail.result.headEventId ?? "",
        };
      } catch (tailErr) {
        if (!isCancelled() && !signal.aborted && agentRuntime().isRunGone(tailErr)) {
          await recoverProjection(signal);
          return null;
        }
        if (isCancelled() || signal.aborted || tailErr instanceof RpcConnectionError) return null;
        throw tailErr;
      }
    };

    if (position.recovery === "cold") return recoverAndTail();
    try {
      const stream = await settleRunStreamOpening(
        client().runs.subscribe(target, signal, {
          ...(position.lastEventId ? { lastEventId: position.lastEventId } : {}),
        }),
        signal,
      );
      if (!stream) return null;
      if (isCancelled() || signal.aborted) {
        retireRunStream(stream);
        return null;
      }
      return {
        result: brandAck(stream.result),
        events: stream.events,
        cursor: position.lastEventId || stream.result.headEventId || "",
      };
    } catch (err) {
      if (isCancelled() || signal.aborted) return null;
      if (agentRuntime().isRunGone(err)) {
        await recoverProjection(signal);
        return null;
      }
      if (err instanceof RpcConnectionError) return null;
      if (!agentRuntime().isReplayLost(err)) {
        console.warn("[agent] run reattach failed:", sessionId, err);
        return null;
      }
      return recoverAndTail();
    }
  };
}

// The subscribe ack is the wire's own shape, so this is the parse site for its ids —
// the same rule the gateway follows for a run it opens.
function brandAck(result: { runId: string; segmentId: string; headEventId?: string }) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    ...(result.headEventId ? { headEventId: result.headEventId } : {}),
  };
}
