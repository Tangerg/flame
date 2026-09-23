import { asRunId, asSegmentId, type FlameClient } from "@/rpc";
import { revalidateAgentSessionMaterial } from "../application/session/refreshSessionProjection";
import { runtimeSessionMaterial } from "./runtimeSessionMaterial";
import { retireRunStream, settleRunStreamOpening } from "./runStreamOpening";

export async function snapshotRunStream(
  client: Pick<FlameClient, "runs">,
  sessionId: string,
  runId: string,
  segmentId: string,
  signal: AbortSignal,
  canCommit: () => boolean,
): Promise<Awaited<ReturnType<FlameClient["runs"]["subscribe"]>> | null> {
  let stream: Awaited<ReturnType<FlameClient["runs"]["subscribe"]>> | null = null;
  try {
    const refreshed = await revalidateAgentSessionMaterial(
      sessionId,
      async () => {
        stream = await settleRunStreamOpening(
          client.runs.subscribe(
            { runId: asRunId(runId), segmentId: asSegmentId(segmentId), snapshot: true },
            signal,
          ),
          signal,
        );
        if (!stream) return null;
        const snapshot = stream.result.snapshot;
        if (!snapshot) throw new Error("agent: snapshot subscription omitted its Session material");
        return runtimeSessionMaterial(sessionId, snapshot);
      },
      { signal, canCommit },
    );
    if (!refreshed?.committed) {
      if (stream) retireRunStream(stream);
      return null;
    }
    return stream;
  } catch (error) {
    if (stream) retireRunStream(stream);
    throw error;
  }
}
