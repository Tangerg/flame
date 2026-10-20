import { type CommandSpec, type ShortcutSpec } from "@/features/command/public/commands";
import { defineExtensionPoint } from "@/extensions/contracts";
import { normalizeCombo } from "@/lib/combo";
export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "flame.shortcut",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

export const COMMAND = defineExtensionPoint<CommandSpec>({
  id: "flame.command",
  keying: "single",
});
