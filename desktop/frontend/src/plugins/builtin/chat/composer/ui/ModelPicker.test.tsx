import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModelPicker } from "./ModelPicker";
import { useRecentModelsStore } from "../adapters/recentModels";

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

function options() {
  return within(screen.getByRole("listbox"));
}

/** The rail, by accessible name — `textContent` would read the count beside each label and
 *  the brand mark's own SVG <title>, neither of which a reader hears. */
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
      model({ provider: "ollama", id: "Mistral Local" }),
      model({ provider: "deepseek", id: "DeepSeek Chat" }),
      model({ provider: "deepseek", id: "DeepSeek Reasoner", reasoning: true }),
    ];
    state.selection = { model: state.models[1]!, reasoningEffort: "high" };
    state.setModel.mockReset();
  });

  afterEach(cleanup);

  it("opens on the provider in force and lists only its models", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));

    await screen.findByPlaceholderText("Search models…");
    // A tab per provider, and only the one holding the current model is listed. The stacked
    // form put every provider in one scroller, so the catalogue grew past the surface as
    // soon as a second provider was configured.
    // By accessible name: the brand mark is `aria-hidden` but carries an SVG <title>, so
    // `textContent` would read the provider twice.
    expectRail(["Ollama", "DeepSeek"]);
    expect(screen.getByRole("button", { name: "DeepSeek", pressed: true })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Ollama", pressed: false })).toBeTruthy();
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      expect.stringContaining("DeepSeek Chat"),
      expect.stringContaining("DeepSeek Reasoner"),
    ]);
  });

  it("holds one measure so the surface does not walk up the screen", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));

    await screen.findByPlaceholderText("Search models…");
    const list = screen.getByRole("listbox");
    expect(list.parentElement!.className).toContain("h-[240px]");
    expect(list.className).toContain("overflow-y-auto");
  });

  it("moves to another provider's models by tab", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    await screen.findByPlaceholderText("Search models…");

    fireEvent.click(screen.getByRole("button", { name: "Ollama" }));
    await waitFor(() => {
      expect(options().getByText("Mistral Local")).toBeTruthy();
      expect(options().queryByText("DeepSeek Chat")).toBeNull();
    });
  });

  it("searches every provider, because a typed name is no longer a tab", async () => {
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    const search = await screen.findByPlaceholderText("Search models…");

    // "Mistral Local" is not in the open tab; the query has to leave the tab behind to find it.
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
      provider: "ollama",
      model: "Mistral Local",
    });

    unmount();
    render(<ModelPicker />);
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    await screen.findByPlaceholderText("Search models…");
    expectRail(["Recent", "Ollama", "DeepSeek"]);
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
});
