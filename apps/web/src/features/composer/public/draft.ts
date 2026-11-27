import { useComposer, type ComposerAccess } from "@/features/composer/public/composer";

export type { ComposerDraftInput } from "@/features/composer/domain/draft";
export { parseComposerSlashCommand } from "@/features/composer/domain/draft";
export type { RequestDraftReplacementConfirmation } from "@/features/composer/application/state";

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
