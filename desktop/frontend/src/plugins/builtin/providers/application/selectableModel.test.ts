import { describe, expect, it } from "vitest";

import { SelectableModel, SelectableModelTokenLimits } from "./selectableModel";

describe("SelectableModel", () => {
  it("owns immutable capability collections", () => {
    const reasoningLevels = ["low", "high"];
    const inputModalities = ["text", "image"];
    const model = new SelectableModel({
      id: "gpt",
      provider: "openai",
      label: "GPT",
      reasoning: true,
      reasoningLevels,
      reasoningDefaultLevel: "high",
      inputModalities,
    });

    reasoningLevels.push("max");
    inputModalities.splice(0);

    expect(model.reasoningLevels).toEqual(["low", "high"]);
    expect(model.inputModalities).toEqual(["text", "image"]);
    expect(Object.isFrozen(model)).toBe(true);
    expect(Object.isFrozen(model.reasoningLevels)).toBe(true);
    expect(Object.isFrozen(model.inputModalities)).toBe(true);
  });

  it("owns modality admission and reasoning fallback behavior", () => {
    const model = new SelectableModel({
      id: "gpt",
      provider: "openai",
      label: "GPT",
      reasoning: true,
      reasoningLevels: ["low", "medium", "high"],
      reasoningDefaultLevel: "medium",
      inputModalities: ["text", "image"],
    });

    expect(model.acceptsInput("image")).toBe(true);
    expect(model.acceptsInput("audio")).toBe(false);
    expect(model.reasoningLevelOrDefault("high")).toBe("high");
    expect(model.reasoningLevelOrDefault("unsupported")).toBe("medium");
  });

  it("rejects contradictory reasoning metadata", () => {
    expect(
      () =>
        new SelectableModel({
          id: "plain",
          provider: "example",
          label: "Plain",
          reasoningLevels: ["high"],
        }),
    ).toThrow(/non-reasoning/);

    const model = new SelectableModel({
      id: "plain",
      provider: "example",
      label: "Plain",
    });

    expect(model.acceptsReasoningLevel("high")).toBe(false);
    expect(model.reasoningLevelOrDefault()).toBeUndefined();
  });

  it("rejects invalid model identity restoration", () => {
    expect(
      () => new SelectableModel({ id: "bad model", provider: "openai", label: "Bad" }),
    ).toThrow("model_identity_not_canonical");
    expect(
      () => new SelectableModel({ id: "gpt", provider: "openai\u0000shadow", label: "Bad" }),
    ).toThrow("provider_identity_not_canonical");
    expect(
      () =>
        new SelectableModel({
          id: "gpt",
          provider: "openai",
          label: "Bad",
          reasoning: true,
          reasoningLevels: ["very high"],
        }),
    ).toThrow("reasoningEffort_identity_not_canonical");
  });

  it("owns token-limit presence without numeric sentinels", () => {
    const limits = new SelectableModelTokenLimits({
      contextWindow: 16_384,
      maxOutputTokens: 32_768,
    });

    expect(limits.contextWindow).toBe(16_384);
    expect(limits.maxInputTokens).toBeUndefined();
    expect(limits.maxOutputTokens).toBe(32_768);
    expect(Object.isFrozen(limits)).toBe(true);
    expect(() => new SelectableModelTokenLimits({})).toThrow(/at least one/);
    expect(() => new SelectableModelTokenLimits({ contextWindow: 0 })).toThrow(/positive/);
  });
});
