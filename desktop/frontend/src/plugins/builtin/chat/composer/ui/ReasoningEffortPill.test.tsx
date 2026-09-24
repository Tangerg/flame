import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SelectableModel } from "@/plugins/builtin/settings/providers/public/queries";
import { ReasoningEffortPill } from "./ReasoningEffortPill";

const state = vi.hoisted(() => ({
  selection: null as { model: unknown; reasoningEffort?: string } | null,
  setModel: vi.fn(),
}));

vi.mock("../public/modelPreference", () => ({
  useSetComposerModelPreference: () => state.setModel,
}));

vi.mock("../public/selectedModel", () => ({
  useSelectedModelSelection: () => state.selection,
}));

function model(reasoningLevels: string[]) {
  return new SelectableModel({
    id: "deepseek-v4-pro",
    provider: "deepseek",
    label: "DeepSeek V4 Pro",
    inputModalities: ["text"],
    outputModalities: ["text"],
    tokenLimits: { contextWindow: 128_000 },
    reasoning: true,
    reasoningLevels,
    ...(reasoningLevels.length > 0 ? { reasoningDefaultLevel: reasoningLevels.at(-2) } : {}),
  });
}

describe("ReasoningEffortPill", () => {
  beforeEach(() => state.setModel.mockReset());
  afterEach(cleanup);

  it("offers exactly the levels the model publishes and sets one without re-choosing it", async () => {
    state.selection = { model: model(["low", "high", "max"]), reasoningEffort: "high" };
    render(<ReasoningEffortPill />);
    const pill = screen.getByRole("button", { name: "Switch reasoning effort" });
    expect(pill.textContent).toContain("High");

    fireEvent.click(pill);
    const items = await screen.findAllByRole("menuitem");
    expect(items.map((item) => item.textContent)).toEqual(["Low", "High", "Max"]);
    fireEvent.click(screen.getByRole("menuitem", { name: "Max" }));
    expect(state.setModel).toHaveBeenCalledWith({
      kind: "explicit",
      provider: "deepseek",
      model: "deepseek-v4-pro",
      reasoningEffort: "max",
    });
  });

  it("falls back to the model default when the remembered effort is not offered", () => {
    state.selection = { model: model(["low", "high", "max"]), reasoningEffort: "medium" };
    render(<ReasoningEffortPill />);
    expect(screen.getByRole("button", { name: "Switch reasoning effort" }).textContent).toContain(
      "High",
    );
  });

  it("is absent for a model that thinks but publishes no levels", () => {
    state.selection = { model: model([]), reasoningEffort: undefined };
    render(<ReasoningEffortPill />);
    expect(screen.queryByRole("button", { name: "Switch reasoning effort" })).toBeNull();
  });
});
