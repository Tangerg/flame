import { useEffect, useEffectEvent } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { type Navigator } from "@/lib/navigation";
import {
  bindAgentSession,
  type AgentSessionDependencies,
} from "@/features/agent/adapters/agentSessionBinding";

export function createActiveSessionObservation(
  navigation: Navigator,
  dependencies: () => AgentSessionDependencies,
  runtimeClient: () => FlameClient,
  onConnectionLost: () => void,
) {
  return function useObserveActiveSession(): void {
    const sessionId = navigation.use((location) => location.session);
    const { agentAccess, projection, sessions, runOpenings } = dependencies();
    const reportConnectionLoss = useEffectEvent(onConnectionLost);
    useEffect(() => {
      if (!sessionId) return;
      return bindAgentSession(
        { agentAccess, projection, sessions, runOpenings },
        runtimeClient,
        sessionId,
        reportConnectionLoss,
      );
    }, [runtimeClient, sessionId, agentAccess, projection, sessions, runOpenings]);
  };
}
