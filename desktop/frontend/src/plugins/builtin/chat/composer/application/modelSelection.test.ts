import { describe, expect, it } from "vitest";
import { resolveComposerModelSelection, resolveComposerRunOptions } from "./modelSelection";

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
    ).toEqual({ model: models[2], reasoningEffort: "high", explicit: true });
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
    ).toEqual({ model: models[1], reasoningEffort: "low", explicit: false });
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
    ).toEqual({ model: models[2], reasoningEffort: "retired-level", explicit: false });
  });

  it("falls back to the target model default when the prior effort is unsupported", () => {
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
    ).toEqual({ model: models[1], reasoningEffort: "high", explicit: true });
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
    ).toEqual({ model: ambiguous[1], reasoningEffort: undefined, explicit: false });
  });

  it("waits for an active Session summary instead of racing to the catalog default", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, undefined)).toBeUndefined();
  });

  it("shows the Runtime default only when no durable Session supplies one", () => {
    expect(resolveComposerModelSelection(models, { kind: "session" }, null)).toEqual({
      model: models[2],
      reasoningEffort: "medium",
      explicit: false,
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
  it("omits an override when the selection follows the Session or Runtime default", () => {
    expect(
      resolveComposerRunOptions(
        resolveComposerModelSelection(
          models,
          { kind: "session" },
          { provider: "openai", model: "gpt-5" },
        ),
      ),
    ).toEqual({});
  });

  it("sends the shown model when an explicit choice falls back to the Session", () => {
    const selection = resolveComposerModelSelection(
      models,
      { kind: "explicit", provider: "anthropic", model: "claude-retired", reasoningEffort: "high" },
      { provider: "openai", model: "gpt-5" },
    );
    expect(selection).toEqual({ model: models[2], reasoningEffort: "medium", explicit: false });
    expect(resolveComposerRunOptions(selection)).toEqual({});
  });

  it("forwards the shown model and effort as one exact override", () => {
    expect(
      resolveComposerRunOptions(
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
      ),
    ).toEqual({ provider: "deepseek", model: "deepseek-v4-pro", reasoningEffort: "high" });
  });
});
