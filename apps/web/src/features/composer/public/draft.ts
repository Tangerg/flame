import { useComposer, type ComposerAccess } from "@/features/composer/public/composer";

import { type ComposerDraftInput } from "@/features/composer/domain/draft";
import {
  type ComposerDraftReplacement,
  type RequestDraftReplacementConfirmation,
} from "@/features/composer/application/state";
export type { ComposerDraftInput } from "@/features/composer/domain/draft";
export type { RequestDraftReplacementConfirmation };

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
  requestConfirmation: RequestDraftReplacementConfirmation,
): void {
  composerAccess.replaceDraft(input, requestConfirmation);
}

export function beginComposerDraftReplacement(
  composerAccess: ComposerAccess,
  requestConfirmation: RequestDraftReplacementConfirmation,
  begin: (replace: ComposerDraftReplacement) => boolean,
): void {
  composerAccess.beginDraftReplacement(requestConfirmation, begin);
}
