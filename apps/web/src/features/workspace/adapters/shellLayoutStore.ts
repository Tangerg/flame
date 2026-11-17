import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { rehydrateOrDefault } from "@/lib/persistedStore";
import { type Paired } from "@/lib/persistedStore";
const STORAGE_KEY = "flame.panel-layout";

interface ShellLayoutState {
  sidebarCollapsed: boolean;
  sidebarLayout: Record<string, number> | null;
  dockLayout: Record<string, number> | null;
}

const shellLayoutPersistSchema = z.object({
  sidebarCollapsed: z.boolean(),
  sidebarLayout: z.record(z.string(), z.number().min(0).max(100)).nullable(),
  dockLayout: z.record(z.string(), z.number().min(0).max(100)).nullable(),
});

const _paired: Paired<ShellLayoutState, z.infer<typeof shellLayoutPersistSchema>> = true;

void _paired;

interface ShellLayoutActions {
  toggleSidebar: () => void;
  saveSidebarLayout: (layout: Record<string, number>) => void;
  saveDockLayout: (layout: Record<string, number>) => void;
}

export function createShellLayoutStore() {
  return create<ShellLayoutState & ShellLayoutActions>()(
    persist(
      (set) => ({
        sidebarCollapsed: false,
        sidebarLayout: null,
        dockLayout: null,

        toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
        saveSidebarLayout: (sidebarLayout) => set({ sidebarLayout }),
        saveDockLayout: (dockLayout) => set({ dockLayout }),
      }),
      {
        name: STORAGE_KEY,
        storage: createJSONStorage(() => localStorage),
        merge: rehydrateOrDefault(STORAGE_KEY, shellLayoutPersistSchema),
      },
    ),
  );
}
export type ShellLayoutStore = ReturnType<typeof createShellLayoutStore>;
