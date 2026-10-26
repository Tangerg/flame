import { type ComposerAccess, useComposer } from "@/features/composer/public/composer";
import { type ComposerModelPreference } from "@/features/composer/application/state";
export type { ComposerModelPreference } from "@/features/composer/application/state";

export function selectedComposerModelPreference(
  composerAccess: ComposerAccess,
): ComposerModelPreference {
  return composerAccess.getModelPreference();
}

export function useComposerModelPreference(): ComposerModelPreference {
  const composerAccess = useComposer();

  return composerAccess.useModelPreference();
}

export function useSetComposerModelPreference(): (preference: ComposerModelPreference) => void {
  const composerAccess = useComposer();

  return composerAccess.useSetModelPreference();
}
