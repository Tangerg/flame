import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModelPicker } from "./ModelPicker";

const state = vi.hoisted(() => ({
  models: [] as ModelFixture[],
  selection: null as { model: ModelFixture; reasoningEffort?: string } | null,
  setModel: vi.fn(),
}));

vi.mock("@/plugins/builtin/settings/providers/public/queries", () => ({
  useModels: () => ({ data: state.models, isLoading: false, isError: false }),
}));

vi.mock("../public/modelPreference", () => ({
  useSetComposerModelPreference: () => state.setModel,
}));

vi.mock("../public/selectedModel", () => ({
  useSelectedModelSelection: () => state.selection,
}));

interface ModelFixture {
  id: string;
  provider: string;
  label: string;
  tokenLimits: { contextWindow: number; maxOutputTokens: number };
  knowledgeCutoff?: string;
  deprecated: boolean;
  reasoning: boolean;
  reasoningLevels: string[];
  reasoningDefaultLevel?: string;
  inputModalities: string[];
  outputModalities: string[];
  toolUse: boolean;
  structuredOutput: boolean;
  reasoningLevelOrDefault: (preferred?: string | null) => string | undefined;
}

function model({
  provider,
  id,
  reasoning = false,
}: {
  provider: string;
  id: string;
  reasoning?: boolean;
}): ModelFixture {
  const reasoningLevels = reasoning ? ["low", "high"] : [];
  return {
    id,
    provider,
    label: id,
    tokenLimits: { contextWindow: 128_000, maxOutputTokens: 8_192 },
    deprecated: false,
    reasoning,
    reasoningLevels,
    reasoningDefaultLevel: reasoning ? "high" : undefined,
    inputModalities: ["text"],
    outputModalities: ["text"],
    toolUse: true,
    structuredOutput: false,
    reasoningLevelOrDefault: (preferred) => {
      if (!reasoning) return undefined;
      return preferred && reasoningLevels.includes(preferred) ? preferred : "high";
    },
  };
}

describe("ModelPicker", () => {
  beforeEach(() => {
    state.models = [
      model({ provider: "ollama", id: "Mistral Local" }),
      model({ provider: "deepseek", id: "DeepSeek Chat" }),
      model({ provider: "deepseek", id: "DeepSeek Reasoner", reasoning: true }),
    ];
    state.selection = { model: state.models[1]!, reasoningEffort: "high" };
    state.setModel.mockReset();
  });

  afterEach(cleanup);

  it("groups the current provider first, filters the catalog, and keeps the list scrollable", async () => {
    render(<ModelPicker />);

    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));

    const search = await screen.findByPlaceholderText("Search models…");
    const options = screen.getAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      expect.stringContaining("DeepSeek Chat"),
      expect.stringContaining("DeepSeek Reasoner"),
      expect.stringContaining("Mistral Local"),
    ]);
    expect(screen.getByRole("listbox").className).toContain("overflow-y-auto");

    fireEvent.change(search, { target: { value: "reasoner" } });
    await waitFor(() => {
      expect(screen.getByText("DeepSeek Reasoner")).toBeTruthy();
      expect(screen.queryByText("Mistral Local")).toBeNull();
    });

    fireEvent.click(screen.getByText("DeepSeek Reasoner"));
    expect(state.setModel).toHaveBeenCalledWith({
      kind: "explicit",
      provider: "deepseek",
      model: "DeepSeek Reasoner",
      reasoningEffort: "high",
    });
  });
});
