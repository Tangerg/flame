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
  synchronizeSelection?: () => void,
): () => void {
  return composerAccess.registerFocusTarget(element, synchronizeSelection);
}

export function registerComposerFocusPresentation(
  composerAccess: ComposerAccess,
  present: (focus: () => void) => void,
): () => void {
  return composerAccess.registerFocusPresentation(present);
}
