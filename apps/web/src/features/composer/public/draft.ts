import { composerState } from "@/features/composer/application/ports/state";
import { type ComposerDraftInput } from "@/features/composer/domain/draft";
export type { ComposerDraftInput } from "@/features/composer/domain/draft";

export function useComposerText(): string {
  return composerState().useText();
}

export function useSetComposerText(): (value: string) => void {
  return composerState().useSetText();
}

export function captureComposerSubmission() {
  return composerState().captureSubmission();
}

export function replaceComposerDraft(input: ComposerDraftInput): void {
  composerState().replaceDraft(input);
}

export function appendToComposerDraft(text: string): void {
  composerState().appendText(text);
}
