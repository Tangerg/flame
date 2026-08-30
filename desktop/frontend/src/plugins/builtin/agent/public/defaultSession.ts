// Switching `activeSessionId` REBUILDS the agent, so the backend sees the new session id on
// the first run.

import { agentDefaultSession, type AgentSession } from "../application/ports/defaultSession";

export type { AgentSession } from "../application/ports/defaultSession";

export function useDefaultChatSession(): AgentSession {
  return agentDefaultSession().useDefaultChatSession();
}
