// WHICH SESSION IS ACTIVE IS NOT HERE — that is the app's location (lib/navigation), so
// history holds it. This store is memory: the tab set, and `lastSessionId`, written as the
// user moves and read once at boot to seed the location. One direction only.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { disposeOnHmr } from "@/lib/hmr";
import { discardOlderVersions } from "@/lib/persistedStore";
import { openSession, pruneDraftSessions } from "../application/session/sessionSelectionModel";

// Mirrors `partialize` below. A malformed entry falls back to defaults rather than
// crashing the boot.
const sessionPersistSchema = z.object({
  lastSessionId: z.string(),
  openSessionIds: z.array(z.string()),
  draftSessionIds: z.array(z.string()),
});

interface AgentSessionState {
  /** Load-bearing lifecycle state: agentStore drops view state, composerStore drops
   *  drafts, and this store drops draft refs for ids no longer in the set. */
  openSessionIds: string[];

  /** Cold-start seed and NOTHING else: reading it to answer "which session is active"
   *  would make it a second owner of the location. */
  lastSessionId: string;

  /** REAL backend sessions created up front so they can receive a run, hidden from the
   *  Work Index until first send. Persisted until graduation so a reload cannot publish an
   *  unused draft as an ordinary Session. */
  draftSessionIds: Set<string>;
  /** Ephemeral, unlike draft ownership: proves an in-process create may skip the first
   *  durable read. */
  freshDraftSessionIds: Set<string>;
}

interface AgentSessionActions {
  holdOpen: (id: string) => void;
  release: (id: string) => void;
  /** Boot reconciliation against the runtime's live ids. */
  retainOnly: (openSessionIds: string[]) => void;
  rememberSession: (id: string) => void;

  markDraft: (id: string) => void;
  graduateDraft: (id: string) => void;
}

export const useAgentSessionStore = create<AgentSessionState & AgentSessionActions>()(
  persist(
    (set, get) => ({
      // Starts empty and is driven by the backend's sessions.list plus user clicks: a ghost
      // id makes the chat load a session the runtime does not have (session_not_found).
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
      // Bump to DISCARD stale payloads rather than migrate (CLAUDE.md §3).
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

// Without this, draft refs grow unbounded and a leftover id makes useAgentSession skip
// history hydration if that id is ever reopened. A live draft is always in openSessionIds
// (holdOpen is paired with selecting it), so "not open" ⇒ dead.
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
