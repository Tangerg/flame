import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { ScopedPersistence } from "@/lib/persistedStore";
import { openSession } from "@/features/agent/application/session/sessionSelectionModel";
const sessionPersistSchema = z.object({
  lastSessionId: z.string(),
  openSessionIds: z.array(z.string()),
});

interface AgentSessionState {
  openSessionIds: string[];

  lastSessionId: string;
}

interface AgentSessionActions {
  holdOpen: (id: string) => void;
  release: (id: string) => void;
  retainOnly: (openSessionIds: string[]) => void;
  rememberSession: (id: string) => void;
}

// One owner for the persisted store key: the ScopedPersistence slot and persist's
// name must agree, or a reload reads from a different slot than it wrote.
const PERSISTENCE_KEY = "flame.agent-session";

export function createAgentSessionStore() {
  const persistence = new ScopedPersistence<AgentSessionState>(PERSISTENCE_KEY);

  function emptySessionState(): AgentSessionState {
    return {
      openSessionIds: [],
      lastSessionId: "",
    };
  }

  const useAgentSessionStore = create<AgentSessionState & AgentSessionActions>()(
    persist(
      (set, get) => ({
        ...emptySessionState(),

        holdOpen: (id) => set({ openSessionIds: openSession(get().openSessionIds, id) }),
        release: (id) =>
          set({ openSessionIds: get().openSessionIds.filter((openId) => openId !== id) }),
        retainOnly: (openSessionIds) => set({ openSessionIds }),
        rememberSession: (id) => set({ lastSessionId: id }),
      }),
      {
        name: PERSISTENCE_KEY,
        storage: createJSONStorage(() => persistence.storage),
        skipHydration: true,
        partialize: (s) => ({
          openSessionIds: s.openSessionIds,
          lastSessionId: s.lastSessionId,
        }),
        onRehydrateStorage: () => (_state, error) => {
          if (error) useAgentSessionStore.setState(emptySessionState());
        },
        merge: (persisted, current) => {
          const defaults = { ...current, ...emptySessionState() };
          if (persisted === undefined) return defaults;
          const parsed = sessionPersistSchema.safeParse(persisted);
          if (!parsed.success) {
            console.warn(
              `[agentSessionStore] discarding corrupted ${PERSISTENCE_KEY}:`,
              parsed.error.issues,
            );
            return defaults;
          }
          return { ...defaults, ...parsed.data };
        },
      },
    ),
  );

  function activateAgentSessionStorage(endpoint: string): boolean {
    return persistence.activate(endpoint, useAgentSessionStore);
  }

  return { store: useAgentSessionStore, activate: activateAgentSessionStorage };
}
export type AgentSessionStore = ReturnType<typeof createAgentSessionStore>;
