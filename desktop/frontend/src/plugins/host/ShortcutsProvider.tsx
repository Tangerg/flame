// Matching is TINYKEYS', not ours: it names the physical key so letter shortcuts survive a
// non-US layout, maps `$mod` to Meta/Control per platform, and treats auto-repeat as one
// intent so holding ⌘N opens one chat rather than thirty.
//
// ONE listener, so a registry change updates one binding map rather than N subscriptions
// and no plugin touches the DOM to own a chord.

import { useEffect } from "react";
import { tinykeys } from "tinykeys";
import { SHORTCUT, useExtensionPoint } from "@/plugins/sdk";
import { dispatchBinding } from "@/lib/combo";

// `allowInInputs: true` opts in so the shortcut fires even in form fields.
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  const tag = target.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
}

export function ShortcutsProvider() {
  const shortcuts = useExtensionPoint(SHORTCUT);

  useEffect(() => {
    const bindings: Record<string, (event: KeyboardEvent) => void> = {};
    for (const spec of shortcuts) {
      // The registry keys this point by canonical combo, so two specs can only
      // collide here if they already collided there — one survived.
      bindings[dispatchBinding(spec.key)] = (event) => {
        if (!spec.allowInInputs && isEditableTarget(event.target)) return;
        spec.handler(event);
      };
    }

    // tinykeys' default `ignore` drops every event targeting a form control, which is the
    // decision `allowInInputs` makes PER shortcut — so it is gated in the handler, leaving
    // only the two rules that hold for all of them: auto-repeat and IME composition.
    return tinykeys(window, bindings, {
      ignore: (event) => event.repeat || event.isComposing,
    });
  }, [shortcuts]);

  return null;
}
