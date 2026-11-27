import { type AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";

export function editAgentSession(
  owner: AgentCommandOwner,
  sessionId: string,
  write: () => Promise<void>,
): Promise<void> {
  return owner.serializeSessionEdit(sessionId, async () => {
    try {
      await write();
    } finally {
      if (owner.isCurrent()) void invalidateAgentSessions();
    }
  });
}
