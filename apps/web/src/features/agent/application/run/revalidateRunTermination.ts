import { type AgentAccess } from "@/features/agent/public/agent";
import { revalidateAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";
export async function revalidateRunTermination(
  agentAccess: AgentAccess,
  sessionId: string,
  runId: string,
): Promise<boolean> {
  const result = await revalidateAgentSessionProjection(agentAccess, sessionId);
  return result?.authoritativeView.runsById[runId]?.status === "finished";
}
