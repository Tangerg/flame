import { normalizeCombo } from "@/lib/combo";
import { isLargePaste } from "@/features/composer/domain/largePaste";
export type ComposerPasteIntent =
  | { kind: "images"; files: File[] }
  | { kind: "large-text"; text: string }
  | { kind: "native" }
  | { kind: "unsupported-images" };

export function composerPasteIntent(
  files: File[],
  text: string,
  acceptsImages: boolean,
): ComposerPasteIntent {
  if (files.length > 0 && acceptsImages) return { kind: "images", files };
  if (files.length > 0 && !text) return { kind: "unsupported-images" };
  if (isLargePaste(text)) return { kind: "large-text", text };
  return { kind: "native" };
}

export function composerKeyBindingKey(
  event: Pick<KeyboardEvent, "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "key">,
): string {
  const parts: string[] = [];
  if (event.metaKey || event.ctrlKey) parts.push("mod");
  if (event.altKey) parts.push("alt");
  if (event.shiftKey) parts.push("shift");
  parts.push(event.key);
  return normalizeCombo(parts.join("+"));
}

export type ComposerSuggestionKeyIntent =
  "dismiss" | "dismiss-and-traverse" | "accept" | "navigate" | null;

export function composerSuggestionKeyIntent(
  event: Pick<KeyboardEvent, "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "key">,
): ComposerSuggestionKeyIntent {
  switch (event.key) {
    case "Escape":
      return "dismiss";
    case "Tab":
      return event.shiftKey ? "dismiss-and-traverse" : "accept";
    case "Enter":
      return event.shiftKey ? null : "accept";
    case "ArrowUp":
    case "ArrowDown":
      return event.shiftKey || event.altKey || event.ctrlKey || event.metaKey ? null : "navigate";
    default:
      return null;
  }
}

export type ComposerCompositionKeyIntent = "active" | "committed-enter" | null;

export function composerCompositionKeyIntent(
  event: Pick<
    KeyboardEvent,
    "altKey" | "ctrlKey" | "isComposing" | "key" | "keyCode" | "metaKey" | "shiftKey"
  >,
  compositionActive: boolean,
  compositionCommitPending: boolean,
): ComposerCompositionKeyIntent {
  if (compositionActive || event.isComposing || event.keyCode === 229) return "active";

  return compositionCommitPending &&
    event.key === "Enter" &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey
    ? "committed-enter"
    : null;
}
