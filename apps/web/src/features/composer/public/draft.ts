import { useComposer, type ComposerAccess } from "@/features/composer/public/composer";

import { type ComposerDraftInput } from "@/features/composer/domain/draft";
export type { ComposerDraftInput } from "@/features/composer/domain/draft";

export function useComposerText(): string {
  const composerAccess = useComposer();

  return composerAccess.useText();
}

export function useSetComposerText(): (value: string) => void {
  const composerAccess = useComposer();

  return composerAccess.useSetText();
}

export function captureComposerSubmission(composerAccess: ComposerAccess) {
  return composerAccess.captureSubmission();
}

export function replaceComposerDraft(
  composerAccess: ComposerAccess,
  input: ComposerDraftInput,
): void {
  composerAccess.replaceDraft(input);
}

export function beginComposerDraftReplacement(composerAccess: ComposerAccess) {
  return composerAccess.beginDraftReplacement();
}

export function appendToComposerDraft(composerAccess: ComposerAccess, text: string): void {
  composerAccess.appendText(text);
}
