import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import { type AgentDefaultSessionPort } from "@/features/agent/application/session/sessionAccess";
import { useAgentSession } from "@/features/agent/adapters/useAgentSession";
import { type AgentSessionDependencies } from "@/features/agent/adapters/agentSessionBinding";
export function createAgentDefaultSession(
  navigation: Navigator,
  dependencies: () => AgentSessionDependencies,
  runtimeClient: () => FlameClient,
  onConnectionLost: () => void,
): AgentDefaultSessionPort {
  function useDefaultChatSession() {
    const activeSessionId = navigation.use((location) => location.session);
    return useAgentSession(dependencies(), runtimeClient, activeSessionId, onConnectionLost);
  }
  return { useDefaultChatSession };
}
