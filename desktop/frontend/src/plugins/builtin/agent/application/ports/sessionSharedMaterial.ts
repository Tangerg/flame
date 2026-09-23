import type { AgentProjectionMaterial } from "./sessionView";
import { agentSessionView } from "./sessionView";

export type AgentSessionSharedMaterialContributor<T> = (sessionId: string, material: T) => unknown;

interface RegisteredContributor {
  active: boolean;
  key: string;
  project(sessionId: string, material: unknown): unknown;
}

const contributors = new Map<string, RegisteredContributor>();

export function registerAgentSessionSharedMaterial<T>(
  key: string,
  project: AgentSessionSharedMaterialContributor<T>,
): () => void {
  if (contributors.has(key)) {
    throw new Error(`Agent Session shared material "${key}" already has an owner`);
  }
  const registered: RegisteredContributor = {
    active: true,
    key,
    project: (sessionId, material) => project(sessionId, material as T),
  };
  contributors.set(key, registered);
  return () => {
    registered.active = false;
    if (contributors.get(key) === registered) contributors.delete(key);
  };
}

export function stageAgentSessionSharedMaterial<T>(
  sessionId: string,
  material: T,
): (shared: Record<string, unknown>) => Record<string, unknown> {
  const staged = [...contributors.values()].map((registered) => ({
    registered,
    value: registered.project(sessionId, material),
  }));
  return (shared) => {
    let projected = shared;
    for (const { registered, value } of staged) {
      if (!registered.active) continue;
      projected = { ...projected, [registered.key]: value };
    }
    return projected;
  };
}

export function useAgentSessionSharedMaterial<T>(path: string): AgentProjectionMaterial<T> {
  return agentSessionView().useSharedMaterial<T>(path);
}

export function getAgentSessionSharedMaterial<T>(sessionId: string, path: string): T | undefined {
  return agentSessionView().getSession(sessionId)?.view.shared[path] as T | undefined;
}
