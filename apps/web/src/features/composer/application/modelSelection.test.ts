import { describe, expect, it } from "vitest";
import {
  resolveComposerModelSelection,
  resolveComposerRunOptions,
} from "@/features/composer/application/modelSelection";
function model(
  provider: string,
  id: string,
  reasoningLevels: readonly string[] = [],
  reasoningDefault?: string,
  runtimeDefault = false,
) {
  return {
    provider,
    id,
    default: runtimeDefault,
    reasoningLevelOrDefault(level?: string | null) {
      if (level && reasoningLevels.includes(level)) return level;
      if (reasoningDefault && reasoningLevels.includes(reasoningDefault)) return reasoningDefault;
      return reasoningLevels[0];
    },
  };
}

const models = [
  model("deepseek", "deepseek-chat"),
  model("deepseek", "deepseek-v4-pro", ["low", "high"], "high"),
  model("openai", "gpt-5", ["low", "medium", "high"], "medium", true),
];

describe("resolveComposerModelSelection", () => {
  it("keeps an explicit model and supported effort across sessions", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "explicit", provider: "openai", model: "gpt-5", reasoningEffort: "high" },
        { provider: "deepseek", model: "deepseek-v4-pro" },
      ),
    ).toEqual({
      provider: "openai",
      modelId: "gpt-5",
      model: models[2],
      reasoningEffort: "high",
    });
  });

  it("restores the active Session exact model and effort before a preference exists", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "session" },
        {
          provider: "deepseek",
          model: "deepseek-v4-pro",
          reasoningEffort: "low",
        },
      ),
    ).toEqual({
      provider: "deepseek",
      modelId: "deepseek-v4-pro",
      model: models[1],
      reasoningEffort: "low",
    });
  });

  it("does not rewrite a durable Session effort retired by a refreshed catalog", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "session" },
        {
          provider: "openai",
          model: "gpt-5",
          reasoningEffort: "retired-level",
        },
      ),
    ).toEqual({
      provider: "openai",
      modelId: "gpt-5",
      model: models[2],
      reasoningEffort: "retired-level",
    });
  });

  it("preserves an authored effort when a refreshed catalog no longer supports it", () => {
    expect(
      resolveComposerModelSelection(
        models,
        {
          kind: "explicit",
          provider: "deepseek",
          model: "deepseek-v4-pro",
          reasoningEffort: "medium",
        },
        null,
      ),
    ).toEqual({
      provider: "deepseek",
      modelId: "deepseek-v4-pro",
      model: models[1],
      reasoningEffort: "medium",
    });
  });

  it("restores a Session by exact provider/model when providers share a model id", () => {
    const ambiguous = [model("provider-a", "shared-model"), model("provider-b", "shared-model")];
    expect(
      resolveComposerModelSelection(
        ambiguous,
        { kind: "session" },
        {
          provider: "provider-b",
          model: "shared-model",
        },
      ),
    ).toEqual({
      provider: "provider-b",
      modelId: "shared-model",
      model: ambiguous[1],
      reasoningEffort: undefined,
    });
  });

  it("waits for an active Session summary instead of racing to the catalog default", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, undefined)).toBeUndefined();
  });

  it("shows the Runtime default only when no durable Session supplies one", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, null)).toEqual({
      provider: "openai",
      modelId: "gpt-5",
      model: models[2],
      reasoningEffort: "medium",
    });
  });

  it("never substitutes another model for a Session's selection the catalog omits", () => {
    expect(
      resolveComposerModelSelection(
        models,
        { kind: "session" },
        { provider: "anthropic", model: "claude-retired" },
      ),
    ).toBeUndefined();
  });

  it("chooses nothing when the catalog does not carry the Runtime default", () => {
    const withoutDefault = models.map((candidate) => ({ ...candidate, default: false }));
    expect(
      resolveComposerModelSelection(withoutDefault, { kind: "session" }, null),
    ).toBeUndefined();
  });
});

describe("resolveComposerRunOptions", () => {
  it("omits overrides while following the Session or Runtime default", () => {
    expect(resolveComposerRunOptions({ kind: "session" })).toEqual({});
  });

  it("preserves the explicit pair and effort even when the catalog omits them", () => {
    const preference = {
      kind: "explicit" as const,
      provider: "anthropic",
      model: "claude-retired",
      reasoningEffort: "retired-level",
    };
    expect(
      resolveComposerModelSelection(models, preference, { provider: "openai", model: "gpt-5" }),
    ).toEqual({
      provider: "anthropic",
      modelId: "claude-retired",
      model: undefined,
      reasoningEffort: "retired-level",
    });
    expect(resolveComposerRunOptions(preference)).toEqual({
      provider: "anthropic",
      model: "claude-retired",
      reasoningEffort: "retired-level",
    });
    expect(resolveComposerModelSelection([], preference, undefined)).toEqual({
      provider: "anthropic",
      modelId: "claude-retired",
      model: undefined,
      reasoningEffort: "retired-level",
    });
  });

  it("forwards the exact authored pair without adding a catalog default effort", () => {
    expect(
      resolveComposerRunOptions({
        kind: "explicit",
        provider: "deepseek",
        model: "deepseek-v4-pro",
      }),
    ).toEqual({ provider: "deepseek", model: "deepseek-v4-pro" });
  });
});
