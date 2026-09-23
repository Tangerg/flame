import { revalidateAgentSessionProjection } from "../session/refreshSessionProjection";

export async function revalidateRunTermination(sessionId: string, runId: string): Promise<boolean> {
  const result = await revalidateAgentSessionProjection(sessionId);
  return result?.authoritativeView.runsById[runId]?.status === "finished";
}
