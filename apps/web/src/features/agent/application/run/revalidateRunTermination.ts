import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { revalidateAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";
export async function revalidateRunTermination(
  agentAccess: AgentDependencies,
  sessionId: string,
  runId: string,
): Promise<boolean> {
  const result = await revalidateAgentSessionProjection(agentAccess, sessionId);
  return result?.authoritativeView.runsById[runId]?.status === "finished";
}
