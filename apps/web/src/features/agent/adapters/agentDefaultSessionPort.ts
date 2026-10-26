import { type FlameClient } from "@flame/runtime-client";
import { pickAgentSource } from "@/features/agent/adapters/runtimeSources";
import { type Navigator } from "@/lib/navigation";
import { type AgentDefaultSessionPort } from "@/features/agent/application/session/sessionAccess";
import {
  useAgentSession,
  type AgentSessionDependencies,
} from "@/features/agent/adapters/useAgentSession";
export function createAgentDefaultSession(
  navigation: Navigator,
  dependencies: () => AgentSessionDependencies,
  runtimeClient: () => FlameClient,
  onConnectionLost: () => void,
): AgentDefaultSessionPort {
  function useDefaultChatSession() {
    const activeSessionId = navigation.use((location) => location.session);
    return useAgentSession(
      dependencies(),
      runtimeClient,
      () => {
        const source = pickAgentSource();
        if (!source) throw new Error("No agent source registered");
        return source.factory();
      },
      activeSessionId,
      onConnectionLost,
    );
  }
  return { useDefaultChatSession };
}
