import { useHotkeys } from "@tanstack/react-hotkeys";
import { normalizeHotkey, parseHotkey } from "@tanstack/hotkeys";
import { useKeymap } from "@/features/command/keymap";

export function ShortcutsProvider() {
  const shortcuts = useKeymap();
  useHotkeys(
    shortcuts.map((spec) => {
      const parsed = parseHotkey(spec.key);
      const commandModified = parsed.ctrl || parsed.meta || parsed.alt;
      return {
        hotkey: normalizeHotkey(spec.key),
        callback: (event: KeyboardEvent) => {
          if (!event.repeat) spec.handler(event);
        },
        options: {
          ignoreInputs: !spec.allowInInputs ? true : !commandModified ? ("auto" as const) : false,
          meta: { name: spec.description },
        },
      };
    }),
    {
      capture: false,
      ignoreDefaultPrevented: true,
      preventDefault: false,
      stopPropagation: false,
    },
  );
  return null;
}
