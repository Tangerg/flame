import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { queryClient } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";
import { MODELS_KEY, SelectableModel } from "@/features/providers/public/queries";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { useSetComposerModelPreference } from "@/features/composer/public/modelPreference";
import { useRecentModelsStore } from "@/features/composer/adapters/recentModels";
import { ModelPicker } from "@/features/composer/ui/ModelPicker";

afterEach(() => {
  cleanup();
  queryClient.clear();
});

const models = [
  new SelectableModel({ provider: "other", id: "first-model", label: "First model" }),
  new SelectableModel({ provider: "openai", id: "kept-model", label: "Kept model" }),
];
const cases = [
  { name: "empty", result: [] as SelectableModel[] },
  { name: "nonempty", result: models },
];

async function mount(read: () => Promise<SelectableModel[]>, explicit = true) {
  const defaults = queryClient.getQueryDefaults([MODELS_KEY]);
  queryClient.setQueryDefaults([MODELS_KEY], { ...defaults, retry: false });
  finishTestClient(() => queryClient.setQueryDefaults([MODELS_KEY], defaults));
  useRecentModelsStore.setState({ recent: [] });
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: MODELS_KEY, fetcher: read });
    ctx.contribute(DATA_PROVIDER, { key: AGENT_SESSIONS_KEY, fetcher: async () => [] });
    ctx.contribute(SETTINGS_PANE, {
      id: "package:chosen:models",
      label: "Model settings",
      intent: "modelProviders",
      component: () => null,
    });
  });
  if (explicit) {
    const { result } = testClient.renderHook(useSetComposerModelPreference);
    act(() => result.current({ kind: "explicit", provider: "openai", model: "kept-model" }));
  }
  testClient.render(
    <>
      <ModelPicker />
      <textarea aria-label="Later draft" defaultValue="Keep this model draft." />
    </>,
  );
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

it.each(cases)("keeps the first paused $name catalog unknown", async ({ result }) => {
  pauseQueries();
  const read = vi.fn(async () => result);
  await mount(read);
  await waitFor(() =>
    expect(queryClient.getQueryState([MODELS_KEY])).toMatchObject({
      status: "pending",
      fetchStatus: "paused",
      data: undefined,
    }),
  );
  const trigger = screen.getByRole("button", { name: "Switch model" });
  expect(trigger.textContent).toContain("kept-model");
  expect(trigger.title).not.toContain("Unavailable in the current catalog");
  fireEvent.click(trigger);
  const search = await screen.findByRole<HTMLInputElement>("combobox", { name: "Search models…" });
  act(() => search.focus());
  fireEvent.change(search, { target: { value: "kept" } });
  expect(screen.getByRole("status").textContent?.replaceAll("\u2060", "")).toBe("Loading");
  expect(screen.queryByText("No models found")).toBeNull();
  expect(screen.queryAllByRole("option")).toHaveLength(0);
  expect(search.getAttribute("aria-activedescendant")).toBeNull();
  expect(read).not.toHaveBeenCalled();

  await act(async () => onlineManager.setOnline(true));
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.data).toEqual(result));
  expect(screen.getByRole("combobox", { name: "Search models…" })).toBe(search);
  expect(search.value).toBe("kept");
  expect(document.activeElement).toBe(search);
  expect(testClient.composer.access.getModelPreference()).toEqual({
    kind: "explicit",
    provider: "openai",
    model: "kept-model",
  });
  expect(useRecentModelsStore.getState().recent).toEqual([]);
});

it("keeps the same search through the first failure, retry and result", async () => {
  const arrival = Promise.withResolvers<SelectableModel[]>();
  const read = vi
    .fn<() => Promise<SelectableModel[]>>()
    .mockRejectedValueOnce(new Error("catalog failed"))
    .mockImplementation(() => arrival.promise);
  await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("error"));
  const trigger = screen.getByRole("button", { name: "Switch model" });
  fireEvent.click(trigger);
  const search = await screen.findByRole<HTMLInputElement>("combobox", { name: "Search models…" });
  fireEvent.change(search, { target: { value: "kept" } });
  expect(screen.getByRole("alert").textContent).toContain("Couldn't load models.");
  expect(screen.queryByText("No models found")).toBeNull();
  expect(trigger.title).not.toContain("Unavailable in the current catalog");
  const refresh = screen.getByRole("button", { name: "Refresh models" });
  act(() => refresh.focus());
  fireEvent.click(refresh);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("pending"));
  expect(screen.getByRole("button", { name: "Refresh models" })).toBe(refresh);
  expect(document.activeElement).toBe(refresh);
  const draft = screen.getByRole("textbox", { name: "Later draft" });
  act(() => draft.focus());
  await act(async () => arrival.resolve(models));
  await screen.findByRole("option", { name: /Kept model/ });
  expect(screen.getByRole("combobox", { name: "Search models…" })).toBe(search);
  expect(search.value).toBe("kept");
  expect(document.activeElement).toBe(draft);
  expect(draft).toHaveProperty("value", "Keep this model draft.");
  expect(read).toHaveBeenCalledTimes(2);
});

it.each(cases)("retains the accepted $name catalog during a paused reread", async ({ result }) => {
  const read = vi.fn(async () => result);
  await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("success"));
  read.mockClear();
  pauseQueries();
  act(() => void queryClient.refetchQueries({ queryKey: [MODELS_KEY] }));
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  await screen.findByRole("combobox", { name: "Search models…" });
  expect(screen.queryByText("Loading")).toBeNull();
  const content = result.length
    ? await screen.findByRole("option", { name: /Kept model/ })
    : await screen.findByText("No models found");
  expect(content).toBeTruthy();
  expect(read).not.toHaveBeenCalled();
});

it("keeps accepted choices beside a failed refresh and retains the current identity", async () => {
  const read = vi
    .fn<() => Promise<SelectableModel[]>>()
    .mockResolvedValueOnce(models)
    .mockRejectedValue(new Error("refresh failed"));
  await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("success"));
  await act(async () => queryClient.refetchQueries({ queryKey: [MODELS_KEY] }));
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  await screen.findByRole("combobox", { name: "Search models…" });
  expect(screen.getByRole("alert").textContent).toContain("Couldn't load models.");
  expect(screen.getByRole("option", { name: /Kept model/ })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Refresh models" })).toBeTruthy();
  expect(testClient.composer.access.getModelPreference()).toEqual({
    kind: "explicit",
    provider: "openai",
    model: "kept-model",
  });
});

it("offers provider configuration after an accepted empty read without choosing a model", async () => {
  await mount(async () => [], false);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("success"));
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  await screen.findByText("No models found");
  fireEvent.click(screen.getByRole("button", { name: "Configure a provider" }));
  expect(testClient.navigation.get().settings).toBe("package:chosen:models");
  expect(testClient.composer.access.getModelPreference().kind).toBe("session");
  expect(useRecentModelsStore.getState().recent).toEqual([]);
});

it("keeps a model search usable before any current model is resolved", async () => {
  const arrival = Promise.withResolvers<SelectableModel[]>();
  await mount(() => arrival.promise, false);
  fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
  const search = await screen.findByRole<HTMLInputElement>("combobox", { name: "Search models…" });
  fireEvent.change(search, { target: { value: "kept" } });
  expect(screen.getByRole("status").textContent?.replaceAll("\u2060", "")).toBe("Loading");
  const draft = screen.getByRole("textbox", { name: "Later draft" });
  act(() => draft.focus());
  await act(async () => arrival.resolve(models));
  const option = await screen.findByRole("option", { name: /Kept model/ });
  expect(screen.getByRole("combobox", { name: "Search models…" })).toBe(search);
  expect(search.value).toBe("kept");
  expect(document.activeElement).toBe(draft);
  fireEvent.click(option);
  expect(testClient.composer.access.getModelPreference()).toEqual({
    kind: "explicit",
    provider: "openai",
    model: "kept-model",
  });
  expect(useRecentModelsStore.getState().recent).toMatchObject([
    { provider: "openai", id: "kept-model" },
  ]);
});
