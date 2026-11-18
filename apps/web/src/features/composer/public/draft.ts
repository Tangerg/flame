import { useComposer, type ComposerAccess } from "@/features/composer/public/composer";

import { type ComposerDraftInput } from "@/features/composer/domain/draft";
export type { ComposerDraftInput } from "@/features/composer/domain/draft";

export function useComposerText(): string {
  const composerAccess = useComposer();

  return composerAccess.useText();
}

export function startComposerFileReference(composerAccess: ComposerAccess): string {
  return composerAccess.startFileReference();
}

export function startComposerSlashCommand(composerAccess: ComposerAccess): string {
  return composerAccess.startSlashCommand();
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
