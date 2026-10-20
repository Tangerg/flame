import {
  agentDefaultSession,
  type AgentSession,
} from "@/features/agent/application/ports/defaultSession";
export type { AgentSession } from "@/features/agent/application/ports/defaultSession";

export function useDefaultChatSession(): AgentSession {
  return agentDefaultSession().useDefaultChatSession();
}
