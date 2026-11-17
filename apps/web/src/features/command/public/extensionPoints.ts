import { type CommandSpec, type ShortcutSpec } from "@/features/command/public/commands";
import { defineExtensionPoint } from "@/extensions/contracts";
import { normalizeHotkey } from "@tanstack/hotkeys";
export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "flame.shortcut",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeHotkey,
});

export const COMMAND = defineExtensionPoint<CommandSpec>({
  id: "flame.command",
  keying: "single",
});
