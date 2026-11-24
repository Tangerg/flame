import { useSyncExternalStore } from "react";

// The client owns which Project its home view shows; the Project itself stays a
// Runtime read.
export interface ProjectHomeTarget {
  current(): string | undefined;
  show(projectId: string): void;
  subscribe(listener: () => void): () => void;
}

export function createProjectHomeTarget(): ProjectHomeTarget {
  let current: string | undefined;
  const listeners = new Set<() => void>();
  return {
    current: () => current,
    show(projectId) {
      if (current === projectId) return;
      current = projectId;
      for (const listener of listeners) listener();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
}

export function useProjectHomeTarget(target: ProjectHomeTarget): string | undefined {
  return useSyncExternalStore(target.subscribe, target.current);
}
