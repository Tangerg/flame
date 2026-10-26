import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentProjectionMaterial } from "@/features/agent/application/session/sessionAccess";

export function useAgentSessionSharedMaterial<T>(path: string): AgentProjectionMaterial<T> {
  return useAgentDependencies().view.useSharedMaterial<T>(path);
}
export type { AgentProjectionMaterial } from "@/features/agent/application/session/sessionAccess";
