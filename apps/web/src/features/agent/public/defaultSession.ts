import { useAgent } from "@/features/agent/public/agent";
import { type AgentSession } from "@/features/agent/application/session/sessionAccess";
export type { AgentSession } from "@/features/agent/application/session/sessionAccess";

export function useDefaultChatSession(): AgentSession {
  const agentAccess = useAgent();

  return agentAccess.defaultSession.useDefaultChatSession();
}
