// Owned by the context that PLAYS the chime, not by the pane that toggles it. A preference
// with a default has to be readable with nothing installed — routing this through the
// settings context's port made the notifier refuse to run unless that pane's plugin had
// loaded first, which is a load order the notifier has no reason to care about.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";

const STORAGE_KEY = "flame.completion-sound";

interface CompletionSoundState {
  completionSound: boolean;
  setCompletionSound: (on: boolean) => void;
}

const persistSchema = z.object({ completionSound: z.boolean() });

export const useCompletionSoundStore = create<CompletionSoundState>()(
  persist(
    (set) => ({
      completionSound: false,
      setCompletionSound: (completionSound) => set({ completionSound }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: rehydrateOrDefault(STORAGE_KEY, persistSchema),
    },
  ),
);
