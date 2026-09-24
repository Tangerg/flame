import { useMemo } from "react";
import type { CommandSpec, ShortcutSpec } from "@/plugins/sdk";
import { SHORTCUT, useEffectiveCommands, useExtensionPoint } from "@/plugins/sdk";
import { dispatchBinding } from "@/lib/combo";

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

function keymapOf(
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
  const commands = useEffectiveCommands();
  const shortcuts = useExtensionPoint(SHORTCUT);
  return useMemo(() => keymapOf(commands, shortcuts), [commands, shortcuts]);
}
