// Read directly, not through a port: a preference with a default must be readable with no
// other plugin installed, or the transcript cannot render until the settings pane loads.

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";

const STREAM_REVEALS = ["smooth", "typewriter"] as const;
export type StreamReveal = (typeof STREAM_REVEALS)[number];

interface StreamRevealState {
  streamReveal: StreamReveal;
  setStreamReveal: (mode: StreamReveal) => void;
}

const STORAGE_KEY = "flame.stream-reveal";

const persistSchema = z.object({ streamReveal: z.enum(STREAM_REVEALS) });

export const useStreamRevealStore = create<StreamRevealState>()(
  persist(
    (set) => ({
      streamReveal: "smooth",
      setStreamReveal: (streamReveal) => set({ streamReveal }),
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
