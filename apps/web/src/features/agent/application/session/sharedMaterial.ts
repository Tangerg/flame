export type AgentSessionSharedMaterialContributor<T> = (sessionId: string, material: T) => unknown;

interface RegisteredContributor {
  active: boolean;
  key: string;
  project(sessionId: string, material: unknown): unknown;
}

export function createAgentSharedMaterial() {
  const contributors = new Map<string, RegisteredContributor>();

  function registerAgentSessionSharedMaterial<T>(
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

  function stageAgentSessionSharedMaterial<T>(
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

  return {
    register: registerAgentSessionSharedMaterial,
    stage: stageAgentSessionSharedMaterial,
    dispose() {
      for (const contributor of contributors.values()) contributor.active = false;
      contributors.clear();
    },
  };
}
export type AgentSharedMaterial = ReturnType<typeof createAgentSharedMaterial>;
