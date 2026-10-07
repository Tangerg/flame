import type { ComposerModelPreference } from "./ports/state";

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
  model: T;
  reasoningEffort?: string;
  explicit: boolean;
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
        explicit: true,
      };
    }
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
          model: sessionModel,
          reasoningEffort:
            activeSessionSelection.reasoningEffort ?? sessionModel.reasoningLevelOrDefault(),
          explicit: false,
        }
      : undefined;
  }
  const runtimeDefault = models.find((candidate) => candidate.default);
  return runtimeDefault
    ? {
        model: runtimeDefault,
        reasoningEffort: runtimeDefault.reasoningLevelOrDefault(),
        explicit: false,
      }
    : undefined;
}

export function resolveComposerRunOptions(
  selection: ResolvedComposerModelSelection<ComposerModelOption> | undefined,
): {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
} {
  if (!selection?.explicit) return {};
  return {
    provider: selection.model.provider,
    model: selection.model.id,
    ...(selection.reasoningEffort ? { reasoningEffort: selection.reasoningEffort } : {}),
  };
}
