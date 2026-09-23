import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
import { discardOlderVersions } from "@/lib/persistedStore";
import { openSession, pruneDraftSessions } from "../application/session/sessionSelectionModel";

const sessionPersistSchema = z.object({
  lastSessionId: z.string(),
  openSessionIds: z.array(z.string()),
  draftSessionIds: z.array(z.string()),
});

interface AgentSessionState {
  openSessionIds: string[];

  lastSessionId: string;

  draftSessionIds: Set<string>;
  freshDraftSessionIds: Set<string>;
}

interface AgentSessionActions {
  holdOpen: (id: string) => void;
  release: (id: string) => void;
  retainOnly: (openSessionIds: string[]) => void;
  rememberSession: (id: string) => void;

  markDraft: (id: string) => void;
  graduateDraft: (id: string) => void;
}

export const useAgentSessionStore = create<AgentSessionState & AgentSessionActions>()(
  persist(
    (set, get) => ({
      openSessionIds: [],
      lastSessionId: "",
      draftSessionIds: new Set<string>(),
      freshDraftSessionIds: new Set<string>(),

      holdOpen: (id) => set({ openSessionIds: openSession(get().openSessionIds, id) }),
      release: (id) =>
        set({ openSessionIds: get().openSessionIds.filter((openId) => openId !== id) }),
      retainOnly: (openSessionIds) => set({ openSessionIds }),
      rememberSession: (id) => set({ lastSessionId: id }),
      markDraft: (id) =>
        set({
          draftSessionIds: new Set(get().draftSessionIds).add(id),
          freshDraftSessionIds: new Set(get().freshDraftSessionIds).add(id),
        }),
      graduateDraft: (id) => {
        const drafts = get().draftSessionIds;
        if (!drafts.has(id)) return;
        const next = new Set(drafts);
        next.delete(id);
        const fresh = new Set(get().freshDraftSessionIds);
        fresh.delete(id);
        set({ draftSessionIds: next, freshDraftSessionIds: fresh });
      },
    }),
    {
      name: "flame.agent-session",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({
        openSessionIds: s.openSessionIds,
        lastSessionId: s.lastSessionId,
        draftSessionIds: [...s.draftSessionIds],
      }),
      version: 7,
      migrate: discardOlderVersions,
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = sessionPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn(
            "[agentSessionStore] discarding corrupted flame.agent-session:",
            parsed.error.issues,
          );
          return current;
        }
        return {
          ...current,
          ...parsed.data,
          draftSessionIds: new Set(parsed.data.draftSessionIds),
        };
      },
    },
  ),
);

const unsubPruneSessionRefs = useAgentSessionStore.subscribe((state, prev) => {
  if (state.openSessionIds === prev.openSessionIds) return;
  const draftSessionIds = pruneDraftSessions(state);
  const open = new Set(state.openSessionIds);
  const freshDraftSessionIds = new Set(
    [...state.freshDraftSessionIds].filter((id) => open.has(id)),
  );
  if (draftSessionIds || freshDraftSessionIds.size !== state.freshDraftSessionIds.size) {
    useAgentSessionStore.setState({
      ...(draftSessionIds ? { draftSessionIds } : {}),
      freshDraftSessionIds,
    });
  }
});
disposeOnHmr(unsubPruneSessionRefs);
