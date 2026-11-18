import { type ComposerAccess } from "@/features/composer/public/composer";

export function focusComposer(composerAccess: ComposerAccess): void {
  composerAccess.focus();
}

export function captureComposerFocus(composerAccess: ComposerAccess): () => void {
  return composerAccess.captureFocus();
}

export function registerComposerFocusPresentation(
  composerAccess: ComposerAccess,
  present: (focus: () => void) => void,
): () => void {
  return composerAccess.registerFocusPresentation(present);
}
