import { refreshAgentSessionProjection } from "./refreshSessionProjection";
import { agentSessionView } from "../ports/sessionView";

const REWRITE_SYNCHRONIZATION_ATTEMPTS = 2;

export async function rehydrateSessionView(sessionId: string): Promise<void> {
  const synchronize = agentSessionView().getSession(sessionId)?.synchronize;
  for (let attempt = 0; attempt < REWRITE_SYNCHRONIZATION_ATTEMPTS; attempt += 1) {
    const committed = synchronize
      ? await synchronize("after-live")
      : Boolean(
          await refreshAgentSessionProjection(sessionId, {
            invalidateQueuedRunEvents: true,
          }),
        );
    if (committed) return;
  }
  throw new Error(`authoritative Session rewrite did not settle for ${sessionId}`);
}
