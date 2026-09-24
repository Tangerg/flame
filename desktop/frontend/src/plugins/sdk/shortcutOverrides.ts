import { useMemo } from "react";
import { z } from "zod";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { normalizeCombo } from "@/lib/combo";
import { discardOlderVersions, rehydrateOrDefault } from "@/lib/persistedStore";
import type { Paired } from "@/lib/persistedStore";
import type { CommandSpec } from "./types";
import { COMMAND } from "./kernelPoints";
import { useExtensionPoint } from "./selectors/extensions";

const STORAGE_KEY = "flame.shortcut-overrides";

interface ShortcutOverridesState {
  overrides: Readonly<Record<string, string | null>>;
  recording: boolean;
  rebind: (commandId: string, combo: string | null) => void;
  reset: (commandId: string) => void;
  resetAll: () => void;
  setRecording: (recording: boolean) => void;
}

const persistSchema = z.object({ overrides: z.record(z.string(), z.string().nullable()) });

const _paired: Paired<
  Pick<ShortcutOverridesState, "overrides">,
  z.infer<typeof persistSchema>
> = true;
void _paired;

export const useShortcutOverrides = create<ShortcutOverridesState>()(
  persist(
    (set) => ({
      overrides: {},
      recording: false,
      rebind: (commandId, combo) =>
        set((state) => ({
          overrides: {
            ...state.overrides,
            [commandId]: combo === null ? null : normalizeCombo(combo),
          },
        })),
      reset: (commandId) =>
        set((state) => {
          const overrides = { ...state.overrides };
          delete overrides[commandId];
          return { overrides };
        }),
      resetAll: () => set({ overrides: {} }),
      setRecording: (recording) => set({ recording }),
    }),
    {
      name: STORAGE_KEY,
      storage: createJSONStorage(() => localStorage),
      version: 1,
      migrate: discardOlderVersions,
      partialize: (state) => ({ overrides: state.overrides }),
      merge: rehydrateOrDefault(STORAGE_KEY, persistSchema),
    },
  ),
);

function effectiveCombo(
  command: Pick<CommandSpec, "id" | "combo">,
  overrides: Readonly<Record<string, string | null>>,
): string | undefined {
  if (Object.hasOwn(overrides, command.id)) return overrides[command.id] ?? undefined;
  return command.combo;
}

export function useEffectiveCommands(): CommandSpec[] {
  const commands = useExtensionPoint(COMMAND);
  const overrides = useShortcutOverrides((state) => state.overrides);
  return useMemo(
    () =>
      commands.map((command) => {
        const combo = effectiveCombo(command, overrides);
        const { combo: _declared, ...rest } = command;
        void _declared;
        return combo === undefined ? rest : { ...rest, combo };
      }),
    [commands, overrides],
  );
}

export function useCommandCombo(commandId: string): string | undefined {
  return useEffectiveCommands().find((command) => command.id === commandId)?.combo;
}
