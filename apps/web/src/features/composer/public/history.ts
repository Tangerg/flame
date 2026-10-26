import { type ComposerAccess } from "@/features/composer/public/composer";

export function recallPreviousComposerHistory(composerAccess: ComposerAccess): boolean {
  return composerAccess.recallPreviousHistory();
}

export function recallNextComposerHistory(composerAccess: ComposerAccess): boolean {
  return composerAccess.recallNextHistory();
}
