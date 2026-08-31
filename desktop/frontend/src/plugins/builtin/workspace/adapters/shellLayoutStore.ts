// How wide the two flanks are and whether the drawer is collapsed. Sits beside the dock
// store because one port reads both: the drawer, the dock's tab set and the dock's width
// are the same layout decision seen from three angles.
//
// The dock width is a RATIO, never a px measure: the px changes with every window resize,
// and CSS re-derives it from the ratio with no React render (`lib/shellGeometry`).

import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions } from "@/lib/persistedStore";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";

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
      name: "flame.shell-layout",
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      merge: (persisted, current) => {
        if (persisted === undefined) return current;
        const parsed = shellLayoutPersistSchema.safeParse(persisted);
        if (!parsed.success) {
          console.warn("[workspace] discarding corrupted flame.shell-layout:", parsed.error.issues);
          return current;
        }
        return { ...current, ...parsed.data };
      },
    },
  ),
);
