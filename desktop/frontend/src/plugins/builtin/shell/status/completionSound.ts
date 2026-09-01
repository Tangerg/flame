// Read directly, not through a port: a preference with a default must be readable with no
// other plugin installed, or the chime cannot fire until the settings pane loads.

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
