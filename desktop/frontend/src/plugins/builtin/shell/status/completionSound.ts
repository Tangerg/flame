// Owned by the context that PLAYS the chime, not by the pane that toggles it. A preference
// with a default has to be readable with nothing installed — routing this through the
// settings context's port made the notifier refuse to run unless that pane's plugin had
// loaded first, which is a load order the notifier has no reason to care about.

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions } from "@/lib/persistedStore";

interface CompletionSoundState {
  completionSound: boolean;
  setCompletionSound: (on: boolean) => void;
}

export const useCompletionSoundStore = create<CompletionSoundState>()(
  persist(
    (set) => ({
      completionSound: false,
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: "flame.completion-sound",
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: (persisted, current) =>
        typeof (persisted as CompletionSoundState | undefined)?.completionSound === "boolean"
          ? { ...current, completionSound: (persisted as CompletionSoundState).completionSound }
          : current,
    },
  ),
);
