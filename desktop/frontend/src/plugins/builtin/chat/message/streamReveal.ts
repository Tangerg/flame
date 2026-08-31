// Owned by the context that RENDERS the reveal — `blockContext.textReveal` is this
// context's type, and the transcript must be able to read the preference with no other
// plugin installed.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions } from "@/lib/persistedStore";

export const STREAM_REVEALS = ["smooth", "typewriter"] as const;
export type StreamReveal = (typeof STREAM_REVEALS)[number];

interface StreamRevealState {
  streamReveal: StreamReveal;
  setStreamReveal: (mode: StreamReveal) => void;
}

const persistSchema = z.object({ streamReveal: z.enum(STREAM_REVEALS) });

export const useStreamRevealStore = create<StreamRevealState>()(
  persist(
    (set) => ({
      streamReveal: "smooth",
      setStreamReveal: (streamReveal) => set({ streamReveal }),
    }),
    {
      name: "flame.stream-reveal",
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: (persisted, current) => {
        const parsed = persistSchema.safeParse(persisted);
        return parsed.success ? { ...current, ...parsed.data } : current;
      },
    },
  ),
);
