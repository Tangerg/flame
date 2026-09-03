import { useMemo } from "react";
import type { CommandSpec, ShortcutSpec } from "@/plugins/sdk";
import { COMMAND, SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { dispatchBinding } from "@/lib/combo";

/**
 * A command's combo IS its registration — nothing else has to list the id, so a command
 * contributed by any plugin at any point in the load order is bound. Command combos are all
 * modifier-led, so they stay live while a field has focus; a shortcut the composer swallows
 * is not global.
 */
export function commandShortcuts(commands: readonly CommandSpec[]): ShortcutSpec[] {
  return commands.flatMap((command) =>
    command.combo === undefined
      ? []
      : [
          {
            key: command.combo,
            description: command.label,
            allowInInputs: true,
            handler: (event: KeyboardEvent) => {
              event.preventDefault();
              void command.run();
            },
          },
        ],
  );
}

/**
 * Every key the application answers, one entry per key. Commands are folded in first, so an
 * explicit shortcut on a key a command already carries wins — the "last registration of a combo
 * wins" rule `ShortcutSpec` states. Identity is the dispatch form, the same string the keydown
 * listener binds, so what this lists and what fires cannot disagree.
 */
export function keymapOf(
  commands: readonly CommandSpec[],
  shortcuts: readonly ShortcutSpec[],
): ShortcutSpec[] {
  const bound = new Map<string, ShortcutSpec>();
  for (const spec of [...commandShortcuts(commands), ...shortcuts]) {
    bound.set(dispatchBinding(spec.key), spec);
  }
  return [...bound.values()];
}

export function useKeymap(): ShortcutSpec[] {
  const commands = useExtensionPoint(COMMAND);
  const shortcuts = useExtensionPoint(SHORTCUT);
  return useMemo(() => keymapOf(commands, shortcuts), [commands, shortcuts]);
}
