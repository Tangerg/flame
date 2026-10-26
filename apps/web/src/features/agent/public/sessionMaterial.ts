import { useAgent } from "@/features/agent/public/agent";

import { type AgentProjectionMaterial } from "@/features/agent/application/session/sessionAccess";
import { type AgentAccess } from "@/features/agent/application/agentAccess";
export function registerAgentSessionSharedMaterial<T>(
  agentAccess: AgentAccess,
  key: string,
  project: (sessionId: string, material: T) => unknown,
): () => void {
  return agentAccess.sharedMaterial.register(key, project);
}
export function useAgentSessionSharedMaterial<T>(path: string): AgentProjectionMaterial<T> {
  const agentAccess = useAgent();

  return agentAccess.view.useSharedMaterial<T>(path);
}

export function getAgentSessionSharedMaterial<T>(
  agentAccess: AgentAccess,
  sessionId: string,
  path: string,
): T | undefined {
  return agentAccess.view.getSession(sessionId)?.view.shared[path] as T | undefined;
}

export type { AgentProjectionMaterial } from "@/features/agent/application/session/sessionAccess";
