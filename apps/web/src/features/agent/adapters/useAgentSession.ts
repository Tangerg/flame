import { useEffect, useEffectEvent } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentSession } from "@/features/agent/application/session/sessionAccess";
import {
  bindAgentSession,
  type AgentSessionDependencies,
} from "@/features/agent/adapters/agentSessionBinding";

export function useAgentSession(
  dependencies: AgentSessionDependencies,
  client: () => FlameClient,
  sessionId: string,
  onConnectionLost: () => void,
): AgentSession {
  const { agentAccess, projection, sessions, runOpenings } = dependencies;
  const reportConnectionLoss = useEffectEvent(onConnectionLost);

  useEffect(() => {
    if (!sessionId) return;
    return bindAgentSession(
      { agentAccess, projection, sessions, runOpenings },
      client,
      sessionId,
      reportConnectionLoss,
    );
  }, [client, sessionId, agentAccess, projection, sessions, runOpenings]);

  return {
    send: (input: AgentInput, options?: AgentRunStartOptions) =>
      projection.store.getState().sessions[sessionId]?.send?.(input, options) ?? null,
    stop: () => {
      projection.store.getState().sessions[sessionId]?.stop?.();
    },
  };
}
