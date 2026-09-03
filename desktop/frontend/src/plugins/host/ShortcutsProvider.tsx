import { useEffect } from "react";
import { tinykeys } from "tinykeys";
import { dispatchBinding } from "@/lib/combo";
import { useKeymap } from "./keymap";

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

export function ShortcutsProvider() {
  const shortcuts = useKeymap();

  useEffect(() => {
    const bindings: Record<string, (event: KeyboardEvent) => void> = {};
    for (const spec of shortcuts) {
      bindings[dispatchBinding(spec.key)] = (event) => {
        if (!spec.allowInInputs && isEditableTarget(event.target)) return;
        spec.handler(event);
      };
    }

    return tinykeys(window, bindings, {
      ignore: (event) => event.repeat || event.isComposing,
    });
  }, [shortcuts]);

  return null;
}
