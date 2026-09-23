import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import type { Paired } from "@/lib/persistedStore";

const STORAGE_KEY = "flame.completion-sound";

interface CompletionSoundState {
  completionSound: boolean;
  setCompletionSound: (on: boolean) => void;
}

const persistSchema = z.object({ completionSound: z.boolean() });

const _paired: Paired<CompletionSoundState, z.infer<typeof persistSchema>> = true;
void _paired;

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
