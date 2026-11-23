import { useMemo } from "react";
import { type CommandSpec, type ShortcutSpec } from "@/features/command/public/commands";
import { SHORTCUT } from "@/features/command/public/extensionPoints";
import { useEffectiveCommands } from "@/features/command/public/shortcutOverrides";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
import { normalizeHotkey } from "@tanstack/hotkeys";
function commandShortcuts(commands: readonly CommandSpec[]): ShortcutSpec[] {
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
    bound.set(normalizeHotkey(spec.key), { ...spec, key: normalizeHotkey(spec.key) });
  }
  return [...bound.values()];
}

export function useKeymap(): ShortcutSpec[] {
  const commands = useEffectiveCommands();
  const shortcuts = useExtensionPoint(SHORTCUT);
  return useMemo(() => keymapOf(commands, shortcuts), [commands, shortcuts]);
}
