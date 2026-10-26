import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { asRunId, asSegmentId, type FlameClient } from "@flame/runtime-client";
import { revalidateAgentSessionMaterial } from "@/features/agent/application/session/refreshSessionProjection";
import { runtimeSessionMaterial } from "@/features/agent/adapters/runtimeSessionMaterial";
import {
  retireRunStream,
  settleRunStreamOpening,
} from "@/features/agent/adapters/runStreamOpening";
export async function snapshotRunStream(
  agentAccess: AgentDependencies,
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
      agentAccess,
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
        return runtimeSessionMaterial(agentAccess.sharedMaterial, sessionId, snapshot);
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
