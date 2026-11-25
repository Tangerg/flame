import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type FlameClient } from "@flame/runtime-client";
import { asRunId, asSegmentId, RpcConnectionError, RpcProtocolError } from "@flame/runtime-client";

import {
  type RunStreamReattachment,
  type RunStreamPosition,
} from "@/features/agent/adapters/agentRunPump";
import {
  retireRunStream,
  settleRunStreamOpening,
} from "@/features/agent/adapters/runStreamOpening";
import { snapshotRunStream } from "@/features/agent/adapters/snapshotRunStream";
interface RunStreamReattachOptions {
  sessionId: string;
  client: () => Pick<FlameClient, "runs">;
  isCancelled: () => boolean;
  recoverProjection: (signal: AbortSignal) => Promise<void>;
  onConnectionLost: () => void;
}

export function createRunStreamReattach(
  agentAccess: AgentDependencies,
  { sessionId, client, isCancelled, recoverProjection, onConnectionLost }: RunStreamReattachOptions,
) {
  return async function reattach(
    position: RunStreamPosition,
    signal: AbortSignal,
  ): Promise<RunStreamReattachment | null> {
    const retired = () => isCancelled() || signal.aborted;
    if (retired()) return null;
    const target = {
      runId: asRunId(position.runId),
      segmentId: asSegmentId(position.segmentId),
    };
    // A retired owner, a lost run or a lost connection ends this reattachment;
    // every other failure belongs to the caller.
    const absorbed = async (error: unknown): Promise<boolean> => {
      if (retired()) return true;
      if (agentAccess.runtime.isRunGone(error)) {
        await recoverProjection(signal);
        return true;
      }
      if (error instanceof RpcConnectionError) {
        onConnectionLost();
        return true;
      }
      return false;
    };
    const recoverAndTail = async (): Promise<RunStreamReattachment | null> => {
      if (retired()) return null;
      try {
        const tail = await snapshotRunStream(
          agentAccess,
          client(),
          sessionId,
          target.runId,
          target.segmentId,
          signal,
          () => !retired(),
        );
        if (!tail) return null;
        if (retired()) {
          retireRunStream(tail);
          return null;
        }
        return {
          result: brandAck(tail.result),
          events: tail.events,
          cursor: tail.result.headEventId ?? null,
        };
      } catch (tailErr) {
        if (await absorbed(tailErr)) return null;
        throw tailErr;
      }
    };

    if (position.recovery === "cold") return recoverAndTail();
    try {
      const stream = await settleRunStreamOpening(
        client().runs.subscribe(target, signal, {
          ...(position.lastEventId !== null ? { lastEventId: position.lastEventId } : {}),
        }),
        signal,
      );
      if (!stream) return null;
      if (retired()) {
        retireRunStream(stream);
        return null;
      }
      return {
        result: brandAck(stream.result),
        events: stream.events,
        cursor: position.lastEventId ?? stream.result.headEventId ?? null,
      };
    } catch (err) {
      if (await absorbed(err)) return null;
      if (err instanceof RpcProtocolError || !agentAccess.runtime.isReplayLost(err)) throw err;
      return recoverAndTail();
    }
  };
}

function brandAck(result: { runId: string; segmentId: string; headEventId?: string }) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    ...(result.headEventId ? { headEventId: result.headEventId } : {}),
  };
}
