import { asRunId, asSegmentId, type FlameClient } from "@flame/runtime-contract/client";
import { agentRuntime } from "../application/ports/runtimeGateway";
import { refreshAgentSessionProjection } from "../application/session/refreshSessionProjection";
import type { RunStream } from "./agentRunPump";
import { retireRunStream } from "./runStreamOpening";
import { snapshotRunStream } from "./snapshotRunStream";

interface ResumeRunStreamOptions {
  client: Pick<FlameClient, "runs">;
  sessionId: string;
  stream: RunStream;
  signal: AbortSignal;
  isCancelled: () => boolean;
}

export async function replaceResumedRunStream({
  client,
  sessionId,
  stream,
  signal,
  isCancelled,
}: ResumeRunStreamOptions): Promise<RunStream | null> {
  retireRunStream(stream);
  const canCommit = () => !signal.aborted && !isCancelled();
  if (!canCommit()) return null;

  let tail: Awaited<ReturnType<typeof snapshotRunStream>>;
  try {
    // Resume commits answers before opening the segment; its acknowledgement has no Item facts.
    tail = await snapshotRunStream(
      client,
      sessionId,
      stream.result.runId,
      stream.result.segmentId,
      signal,
      canCommit,
    );
  } catch (error) {
    if (!canCommit()) return null;
    if (!agentRuntime().isRunGone(error)) throw error;
    await refreshAgentSessionProjection(sessionId, { signal, canCommit });
    return null;
  }
  if (!tail) return null;
  return {
    result: {
      ...tail.result,
      runId: asRunId(tail.result.runId),
      segmentId: asSegmentId(tail.result.segmentId),
    },
    events: tail.events,
  };
}
