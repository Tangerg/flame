import { describe, expect, it } from "vitest";
import {
  SelectableModel,
  SelectableModelTokenLimits,
} from "@/features/providers/application/selectableModel";
describe("SelectableModel", () => {
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

  it("owns token-limit presence without numeric sentinels", () => {
    const limits = new SelectableModelTokenLimits({
      contextWindow: 16_384,
      maxOutputTokens: 32_768,
    });

    expect(limits.contextWindow).toBe(16_384);
    expect(limits.maxInputTokens).toBeUndefined();
    expect(limits.maxOutputTokens).toBe(32_768);
    expect(Object.isFrozen(limits)).toBe(true);
  });
});
