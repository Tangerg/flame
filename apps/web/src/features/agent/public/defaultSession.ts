import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentSession } from "@/features/agent/application/session/sessionAccess";
export type { AgentSession } from "@/features/agent/application/session/sessionAccess";

export function useDefaultChatSession(): AgentSession {
  const agentAccess = useAgentDependencies();

  return agentAccess.defaultSession.useDefaultChatSession();
}
