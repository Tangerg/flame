import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import type { Paired } from "@/lib/persistedStore";

const STORAGE_KEY = "flame.system-notifications";

interface SystemNotificationsState {
  systemNotifications: boolean;
  setSystemNotifications: (on: boolean) => void;
}

const persistSchema = z.object({ systemNotifications: z.boolean() });

const _paired: Paired<SystemNotificationsState, z.infer<typeof persistSchema>> = true;
void _paired;

export const useSystemNotificationsStore = create<SystemNotificationsState>()(
  persist(
    (set) => ({
      systemNotifications: true,
      setSystemNotifications: (systemNotifications) => set({ systemNotifications }),
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
