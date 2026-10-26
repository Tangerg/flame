import { type AgentAccess } from "@/features/agent/public/agent";
import { refreshAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";

const REWRITE_SYNCHRONIZATION_ATTEMPTS = 2;

export async function rehydrateSessionView(
  agentAccess: AgentAccess,
  sessionId: string,
): Promise<void> {
  const synchronize = agentAccess.view.getSession(sessionId)?.synchronize;
  for (let attempt = 0; attempt < REWRITE_SYNCHRONIZATION_ATTEMPTS; attempt += 1) {
    const committed = synchronize
      ? await synchronize("after-live")
      : Boolean(
          await refreshAgentSessionProjection(agentAccess, sessionId, {
            invalidateQueuedRunEvents: true,
          }),
        );
    if (committed) return;
  }
  throw new Error(`authoritative Session rewrite did not settle for ${sessionId}`);
}
