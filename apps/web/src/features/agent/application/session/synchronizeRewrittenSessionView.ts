import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { refreshAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";

export async function synchronizeRewrittenSessionView(
  agentAccess: AgentDependencies,
  sessionId: string,
): Promise<boolean> {
  const entry = agentAccess.view.getSession(sessionId);
  if (!entry) return false;
  const committed = entry.synchronize
    ? await entry.synchronize("after-live")
    : Boolean(
        await refreshAgentSessionProjection(agentAccess, sessionId, { queued: "invalidate" }),
      );
  if (committed || !agentAccess.view.getSession(sessionId)) return committed;
  throw new Error(`could not refresh rewritten Session ${sessionId}`);
}
