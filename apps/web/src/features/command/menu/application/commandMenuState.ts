import { create } from "zustand";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { COMMAND } from "@/features/command/public/extensionPoints";
import type { CommandMenuAction, CommandSpec } from "@/features/command/public/commands";

interface CommandMenuState {
  preparedActions: ReadonlyMap<CommandSpec, CommandMenuAction> | null;
  setOpen: (open: boolean) => void;
  toggle: () => void;
}

export const useCommandMenuStore = create<CommandMenuState>((set, get) => ({
  preparedActions: null,
  setOpen(open) {
    if (open === (get().preparedActions !== null)) return;
    if (!open) {
      set({ preparedActions: null });
      return;
    }
    const preparedActions = new Map<CommandSpec, CommandMenuAction>();
    for (const command of lookupExtensionPoint(COMMAND)) {
      const action = command.prepareForMenu?.();
      if (action) preparedActions.set(command, action);
    }
    set({ preparedActions });
  },
  toggle: () => get().setOpen(get().preparedActions === null),
}));
