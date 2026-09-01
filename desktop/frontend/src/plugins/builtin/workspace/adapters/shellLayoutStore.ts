// The dock width is a RATIO, never a px measure: the px changes with every window resize,
// and CSS re-derives it from the ratio with no React render (`lib/shellGeometry`).

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";

const STORAGE_KEY = "flame.shell-layout";

interface ShellLayoutState {
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  dockWidthRatio: number | null;
}

const shellLayoutPersistSchema = z.object({
  sidebarCollapsed: z.boolean(),
  sidebarWidth: z.number(),
  dockWidthRatio: z.number().min(0).max(1).nullable(),
});

interface ShellLayoutActions {
  toggleSidebar: () => void;
  setSidebarWidth: (width: number) => void;
  setDockWidthRatio: (ratio: number | null) => void;
}

export const useShellLayoutStore = create<ShellLayoutState & ShellLayoutActions>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
      dockWidthRatio: null,

      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarWidth: (sidebarWidth) => set({ sidebarWidth }),
      setDockWidthRatio: (dockWidthRatio) => set({ dockWidthRatio }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: rehydrateOrDefault(STORAGE_KEY, shellLayoutPersistSchema),
    },
  ),
);
