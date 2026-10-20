import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModelPicker } from "@/features/composer/ui/ModelPicker";
import { useRecentModelsStore } from "@/features/composer/adapters/recentModels";
const state = vi.hoisted(() => ({
  models: [] as ModelFixture[],
  selection: null as {
    provider: string;
    modelId: string;
    model: ModelFixture | undefined;
    reasoningEffort?: string;
  } | null,
  setModel: vi.fn(),
}));

vi.mock("@/features/providers/public/queries", () => ({
  useModels: () => ({ data: state.models, isLoading: false, isError: false }),
}));

vi.mock("@/features/composer/public/modelPreference", () => ({
  useSetComposerModelPreference: () => state.setModel,
}));

vi.mock("@/features/composer/public/selectedModel", () => ({
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

function options() {
  return within(screen.getByRole("listbox"));
}

function expectRail(names: string[]) {
  const rail = screen
    .getAllByRole("button")
    .filter((node) => node.getAttribute("aria-pressed") !== null);
  expect(rail).toHaveLength(names.length);
  for (const name of names) {
    expect(screen.getByRole("button", { name })).toBeTruthy();
  }
}

describe("ModelPicker", () => {
  beforeEach(() => {
    useRecentModelsStore.setState({ recent: [] });
    state.models = [
      model({ provider: "mistral", id: "Mistral Local" }),
      model({ provider: "deepseek", id: "DeepSeek Chat" }),
      model({ provider: "deepseek", id: "DeepSeek Reasoner", reasoning: true }),
    ];
    state.selection = {
      provider: state.models[1]!.provider,
      modelId: state.models[1]!.id,
      model: state.models[1]!,
      reasoningEffort: "high",
    };
    state.setModel.mockReset();
  });

  afterEach(cleanup);

  it("shows an explicit identity as unavailable while an incomplete catalog omits it", () => {
    state.models = [];
    state.selection = {
      provider: "custom",
      modelId: "authored-model",
      model: undefined,
      reasoningEffort: "authored-effort",
    };
    const { rerender } = render(<ModelPicker />);
    const selected = screen.getByRole("button", { name: "Switch model" });
    expect(selected.textContent).toContain("authored-model");
    expect(selected.title).toContain("Unavailable in the current catalog");
    state.models = [model({ provider: "other", id: "other-model" })];
    rerender(<ModelPicker />);
    expect(screen.getByRole("button", { name: "Switch model" }).textContent).toContain(
      "authored-model",
    );
    expect(state.setModel).not.toHaveBeenCalled();
  });

  it("opens on the provider in force and lists only its models", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));

    await screen.findByPlaceholderText("Search models…");
    expectRail(["Mistral", "DeepSeek"]);
    expect(screen.getByRole("button", { name: "DeepSeek", pressed: true })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Mistral", pressed: false })).toBeTruthy();
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      expect.stringContaining("DeepSeek Chat"),
      expect.stringContaining("DeepSeek Reasoner"),
    ]);
  });

  it("moves to another provider's models by tab", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    await screen.findByPlaceholderText("Search models…");

    fireEvent.click(screen.getByRole("button", { name: "Mistral" }));
    await waitFor(() => {
      expect(options().getByText("Mistral Local")).toBeTruthy();
      expect(options().queryByText("DeepSeek Chat")).toBeNull();
    });
  });

  it("searches every provider, because a typed name is no longer a tab", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    const search = await screen.findByPlaceholderText("Search models…");

    fireEvent.change(search, { target: { value: "mistral" } });
    await waitFor(() => {
      expect(options().getByText("Mistral Local")).toBeTruthy();
      expect(options().queryByText("DeepSeek Chat")).toBeNull();
    });
  });

  it("gives the query back before it gives up the surface", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    const search = await screen.findByPlaceholderText("Search models…");

    fireEvent.change(search, { target: { value: "mistral" } });
    await waitFor(() => expect(options().getByText("Mistral Local")).toBeTruthy());

    fireEvent.keyDown(search, { key: "Escape" });
    await waitFor(() => {
      expect(options().getByText("DeepSeek Chat")).toBeTruthy();
    });
    expect(screen.getByPlaceholderText("Search models…")).toBeTruthy();
  });

  it("shelves what was chosen, so the next pick is one tab away", async () => {
    const { unmount } = render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    const search = await screen.findByPlaceholderText("Search models…");

    fireEvent.change(search, { target: { value: "mistral" } });
    await waitFor(() => expect(options().getByText("Mistral Local")).toBeTruthy());
    fireEvent.click(options().getByText("Mistral Local"));
    expect(state.setModel).toHaveBeenCalledWith({
      kind: "explicit",
      provider: "mistral",
      model: "Mistral Local",
    });

    unmount();
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    await screen.findByPlaceholderText("Search models…");
    expectRail(["Recent", "Mistral", "DeepSeek"]);
  });

  it("carries the reasoning effort the selection was already holding", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    const search = await screen.findByPlaceholderText("Search models…");

    fireEvent.change(search, { target: { value: "reasoner" } });
    await waitFor(() => expect(options().getByText("DeepSeek Reasoner")).toBeTruthy());
    fireEvent.click(options().getByText("DeepSeek Reasoner"));
    expect(state.setModel).toHaveBeenCalledWith({
      kind: "explicit",
      provider: "deepseek",
      model: "DeepSeek Reasoner",
      reasoningEffort: "high",
    });
  });

  it("names only the model; effort has its own control", () => {
    state.selection = {
      provider: state.models[2]!.provider,
      modelId: state.models[2]!.id,
      model: state.models[2]!,
      reasoningEffort: "high",
    };
    render(<ModelPicker />);
    const label = screen
      .getByRole("button", { name: "Switch model" })
      .querySelector('[data-slot="composer-chip-label"]');
    expect(label?.textContent).toBe("DeepSeek Reasoner");
  });

  it("marks the model in use by its row, without a radio column", async () => {
    state.selection = {
      provider: state.models[2]!.provider,
      modelId: state.models[2]!.id,
      model: state.models[2]!,
      reasoningEffort: "high",
    };
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    await screen.findByPlaceholderText("Search models…");
    const current = options().getByRole("option", { name: /DeepSeek Reasoner/ });
    expect(current.hasAttribute("data-current")).toBe(true);
    expect(screen.queryByRole("button", { name: "Switch reasoning effort" })).toBeNull();
  });
});
