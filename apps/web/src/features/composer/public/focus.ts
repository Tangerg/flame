import { type ComposerAccess } from "@/features/composer/public/composer";

export function focusComposer(composerAccess: ComposerAccess, selectionEnd?: number): void {
  composerAccess.focus(selectionEnd);
}

export function captureComposerFocus(composerAccess: ComposerAccess): () => void {
  return composerAccess.captureFocus();
}

export function registerComposerFocusTarget(
  composerAccess: ComposerAccess,
  element: HTMLTextAreaElement | null,
): () => void {
  return composerAccess.registerFocusTarget(element);
}

export function registerComposerFocusPresentation(
  composerAccess: ComposerAccess,
  present: (focus: () => void) => void,
): () => void {
  return composerAccess.registerFocusPresentation(present);
}
