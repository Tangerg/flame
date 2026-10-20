import { composerState } from "@/features/composer/application/ports/state";
export function focusComposer(selectionEnd?: number): void {
  composerState().focus(selectionEnd);
}

export function captureComposerFocus(): () => void {
  return composerState().captureFocus();
}

export function registerComposerFocusTarget(element: HTMLTextAreaElement | null): () => void {
  return composerState().registerFocusTarget(element);
}
