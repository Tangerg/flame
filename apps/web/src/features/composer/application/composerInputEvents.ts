import { normalizeHotkey, parseKeyboardEvent } from "@tanstack/hotkeys";
import { isLargePaste } from "@/features/composer/domain/largePaste";
type ComposerPasteIntent =
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

export function composerKeyBindingKey(event: KeyboardEvent): string {
  return normalizeHotkey(parseKeyboardEvent(event));
}
