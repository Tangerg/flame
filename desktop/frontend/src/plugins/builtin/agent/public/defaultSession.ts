import { agentDefaultSession, type AgentSession } from "../application/ports/defaultSession";

export type { AgentSession } from "../application/ports/defaultSession";

export function useDefaultChatSession(): AgentSession {
  return agentDefaultSession().useDefaultChatSession();
}
