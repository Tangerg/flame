import type { ComposerModelPreference } from "./ports/state";

export interface ComposerModelOption {
  id: string;
  provider: string;
  reasoningLevelOrDefault(level?: string | null): string | undefined;
}

export interface ComposerSessionModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface ResolvedComposerModelSelection<T extends ComposerModelOption> {
  model: T;
  reasoningEffort?: string;
}

export function resolveComposerModelSelection<T extends ComposerModelOption>(
  models: readonly T[],
  preference: ComposerModelPreference,
  activeSessionSelection: ComposerSessionModelSelection | null | undefined,
): ResolvedComposerModelSelection<T> | undefined {
  if (preference.kind === "explicit") {
    const preferred = models.find(
      (candidate) =>
        candidate.provider === preference.provider && candidate.id === preference.model,
    );
    if (preferred) {
      return {
        model: preferred,
        reasoningEffort: preferred.reasoningLevelOrDefault(preference.reasoningEffort),
      };
    }
  }
  if (activeSessionSelection === undefined) return undefined;
  if (activeSessionSelection !== null) {
    const sessionModel = models.find(
      (candidate) =>
        candidate.provider === activeSessionSelection.provider &&
        candidate.id === activeSessionSelection.model,
    );
    if (sessionModel) {
      return {
        model: sessionModel,
        reasoningEffort:
          activeSessionSelection.reasoningEffort ?? sessionModel.reasoningLevelOrDefault(),
      };
    }
  }
  const fallback = models[0];
  return fallback
    ? { model: fallback, reasoningEffort: fallback.reasoningLevelOrDefault() }
    : undefined;
}

export function resolveComposerRunOptions(preference: ComposerModelPreference): {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
} {
  return preference.kind === "explicit"
    ? {
        provider: preference.provider,
        model: preference.model,
        ...(preference.reasoningEffort ? { reasoningEffort: preference.reasoningEffort } : {}),
      }
    : {};
}
