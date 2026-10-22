import { useEffect } from "react";
import { tinykeys } from "tinykeys";
import { dispatchBinding, normalizeCombo } from "@/lib/combo";
import { useShortcutOverrides } from "@/features/command/public/shortcutOverrides";
import { useKeymap } from "@/features/command/keymap";
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

const COMMAND_MODIFIERS = new Set(["mod", "ctrl", "alt", "meta"]);

// A bare printable key (or Space) types a character; it must never fire a
// command inside a field even when the command is allowInInputs. Only a
// command-modified combo (Mod+K) or a non-typing bare key (Escape, arrows) is.
function firesInEditable(key: string): boolean {
  const parts = normalizeCombo(key).split("+");
  if (parts.some((part) => COMMAND_MODIFIERS.has(part))) return true;
  const base = parts.at(-1) ?? "";
  return base.length > 1 && base !== "space";
}

export function ShortcutsProvider() {
  const shortcuts = useKeymap();

  useEffect(() => {
    const bindings: Record<string, (event: KeyboardEvent) => void> = {};
    for (const spec of shortcuts) {
      bindings[dispatchBinding(spec.key)] = (event) => {
        if (useShortcutOverrides.getState().recording) return;
        if (isEditableTarget(event.target) && (!spec.allowInInputs || !firesInEditable(spec.key)))
          return;
        spec.handler(event);
      };
    }

    return tinykeys(window, bindings, {
      ignore: (event) => event.repeat || event.isComposing,
    });
  }, [shortcuts]);

  return null;
}
