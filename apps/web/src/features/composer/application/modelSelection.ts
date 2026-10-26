import { type ComposerModelPreference } from "@/features/composer/application/state";
export interface ComposerModelOption {
  id: string;
  provider: string;
  default: boolean;
  reasoningLevelOrDefault(level?: string | null): string | undefined;
}

export interface ComposerSessionModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface ResolvedComposerModelSelection<T extends ComposerModelOption> {
  provider: string;
  modelId: string;
  model: T | undefined;
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
    return {
      provider: preference.provider,
      modelId: preference.model,
      model: preferred,
      reasoningEffort: preference.reasoningEffort,
    };
  }
  if (activeSessionSelection === undefined) return undefined;
  // An implicit selection is whatever the Runtime will run: the Session's own
  // selection, or the Runtime default for the Session a send creates. A model
  // an incomplete catalog omits is reported as unknown, never replaced.
  if (activeSessionSelection !== null) {
    const sessionModel = models.find(
      (candidate) =>
        candidate.provider === activeSessionSelection.provider &&
        candidate.id === activeSessionSelection.model,
    );
    return sessionModel
      ? {
          provider: activeSessionSelection.provider,
          modelId: activeSessionSelection.model,
          model: sessionModel,
          reasoningEffort:
            activeSessionSelection.reasoningEffort ?? sessionModel.reasoningLevelOrDefault(),
        }
      : undefined;
  }
  const runtimeDefault = models.find((candidate) => candidate.default);
  return runtimeDefault
    ? {
        provider: runtimeDefault.provider,
        modelId: runtimeDefault.id,
        model: runtimeDefault,
        reasoningEffort: runtimeDefault.reasoningLevelOrDefault(),
      }
    : undefined;
}

export function resolveComposerRunOptions(preference: ComposerModelPreference): {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
} {
  if (preference.kind !== "explicit") return {};
  return {
    provider: preference.provider,
    model: preference.model,
    ...(preference.reasoningEffort ? { reasoningEffort: preference.reasoningEffort } : {}),
  };
}
