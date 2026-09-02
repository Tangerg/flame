import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";

/** A model the reader has actually chosen, most recent first. Identity is the pair, because
 *  two providers may serve the same id. */
export interface RecentModel {
  provider: string;
  id: string;
}

// Long enough that the shelf answers "the two or three I move between" without becoming a
// second copy of the catalogue.
const KEEP = 5;
const STORAGE_KEY = "flame.composer.recent-models";

const persistSchema = z.object({
  recent: z.array(z.object({ provider: z.string(), id: z.string() })).max(KEEP),
});

interface RecentModelsState {
  recent: RecentModel[];
  remember: (model: RecentModel) => void;
}

export const useRecentModelsStore = create<RecentModelsState>()(
  persist(
    (set, get) => ({
      recent: [],
      remember(model) {
        const without = get().recent.filter(
          (entry) => entry.provider !== model.provider || entry.id !== model.id,
        );
        set({ recent: [model, ...without].slice(0, KEEP) });
      },
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
