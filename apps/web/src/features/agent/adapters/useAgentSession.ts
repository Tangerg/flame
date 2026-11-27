import { useEffect, useEffectEvent } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentSession } from "@/features/agent/application/session/sessionAccess";
import { type RuntimeAgentDriver } from "@/features/agent/adapters/runtimeDriver";
import {
  bindAgentSession,
  type AgentSessionDependencies,
} from "@/features/agent/adapters/agentSessionBinding";

export function useAgentSession(
  dependencies: AgentSessionDependencies,
  client: () => FlameClient,
  makeDriver: () => RuntimeAgentDriver,
  sessionId: string,
  onConnectionLost: () => void,
): AgentSession {
  const { agentAccess, projection, sessions, retries } = dependencies;
  const createDriver = useEffectEvent(makeDriver);
  const reportConnectionLoss = useEffectEvent(onConnectionLost);

  useEffect(() => {
    if (!sessionId) return;
    return bindAgentSession(
      { agentAccess, projection, sessions, retries },
      client,
      createDriver(),
      sessionId,
      reportConnectionLoss,
    );
  }, [client, sessionId, agentAccess, projection, sessions, retries]);

  return {
    send: (input: AgentInput, options?: AgentRunStartOptions) =>
      projection.store.getState().sessions[sessionId]?.send?.(input, options) ?? null,
    stop: () => {
      projection.store.getState().sessions[sessionId]?.stop?.();
    },
  };
}
