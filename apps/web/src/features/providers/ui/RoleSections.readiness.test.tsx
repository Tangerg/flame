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
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
  ProviderConfiguration,
  SelectableModel,
  type ProviderRole,
} from "@/features/providers/application/providerQueries";
import { ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
import { type ProviderGateway } from "@/features/providers/application/ports/providerGateway";
import { EmbeddingModelSection, UtilityModelSection } from "@/features/providers/ui/RoleSections";

let owner: ProviderMutationOwner | undefined;
afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

const role: ProviderRole = { provider: "openai", model: "kept-model" };
const providers = [ProviderConfiguration.restore({ id: "openai", configured: true })];
const models = [new SelectableModel({ provider: "openai", id: "kept-model", label: "Kept model" })];
const cases = [
  { name: "empty", result: [] as SelectableModel[] },
  { name: "nonempty", result: models },
];

async function mount(read: () => Promise<SelectableModel[]>) {
  const defaults = queryClient.getQueryDefaults([MODELS_KEY]);
  queryClient.setQueryDefaults([MODELS_KEY], { ...defaults, retry: false });
  finishTestClient(() => queryClient.setQueryDefaults([MODELS_KEY], defaults));
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: MODELS_KEY, fetcher: read });
    ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => providers });
    ctx.contribute(DATA_PROVIDER, { key: UTILITY_ROLE_KEY, fetcher: async () => role });
    ctx.contribute(DATA_PROVIDER, { key: EMBEDDING_ROLE_KEY, fetcher: async () => ({}) });
  });
  for (const [key, data] of [
    [PROVIDERS_KEY, providers],
    [UTILITY_ROLE_KEY, role],
    [EMBEDDING_ROLE_KEY, {}],
  ] as const)
    queryClient.setQueryData([key], data);
  const gateway = {
    updateProvider: vi.fn<ProviderGateway["updateProvider"]>(),
    testProvider: vi.fn<ProviderGateway["testProvider"]>(),
    setUtilityRole: vi.fn<ProviderGateway["setUtilityRole"]>(),
    setEmbeddingRole: vi.fn<ProviderGateway["setEmbeddingRole"]>(),
    errorMessage: () => undefined,
  } satisfies ProviderGateway;
  owner = ProviderMutationOwner.install(gateway);
  testClient.render(
    <>
      <UtilityModelSection />
      <EmbeddingModelSection />
      <textarea aria-label="Later draft" defaultValue="Keep this role draft." />
    </>,
  );
  return gateway;
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

it.each(cases)(
  "keeps known roles independent of the first paused $name choices",
  async ({ result }) => {
    pauseQueries();
    const read = vi.fn(async () => result);
    const gateway = await mount(read);
    await waitFor(() =>
      expect(queryClient.getQueryState([MODELS_KEY])).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      }),
    );
    const trigger = screen.getByRole("button", { name: "Utility model" });
    expect(trigger.textContent).toContain("kept-model");
    expect(screen.getByRole("button", { name: "Embedding model" }).textContent).toBe("Off");
    fireEvent.click(trigger);
    expect(await screen.findByRole("menuitemradio", { name: "Use Runtime default" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(read).not.toHaveBeenCalled();
    const draft = screen.getByRole("textbox", { name: "Later draft" });
    act(() => draft.focus());
    await act(async () => onlineManager.setOnline(true));
    await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.data).toEqual(result));
    expect(document.activeElement).toBe(draft);
    expect(draft).toHaveProperty("value", "Keep this role draft.");
    expect(queryClient.getQueryData([UTILITY_ROLE_KEY])).toEqual(role);
    expect(gateway.setUtilityRole).not.toHaveBeenCalled();
    expect(gateway.setEmbeddingRole).not.toHaveBeenCalled();
  },
);

it("recovers a failed choice read from its own menu without changing the role", async () => {
  const arrival = Promise.withResolvers<SelectableModel[]>();
  const read = vi
    .fn<() => Promise<SelectableModel[]>>()
    .mockRejectedValueOnce(new Error("catalog failed"))
    .mockImplementation(() => arrival.promise);
  const gateway = await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("error"));
  const trigger = screen.getByRole("button", { name: "Utility model" });
  fireEvent.click(trigger);
  expect(await screen.findByRole("menuitemradio", { name: "Use Runtime default" })).toBeTruthy();
  const menu = screen.getByRole("menu");
  expect(menu.textContent).toContain("Couldn't load models.");
  const refresh = screen.getByRole("menuitem", { name: "Refresh models" });
  act(() => refresh.focus());
  fireEvent.click(refresh);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("pending"));
  expect(screen.getByRole("menuitem", { name: "Refresh models" })).toBe(refresh);
  expect(document.activeElement).toBe(refresh);
  await act(async () => arrival.resolve(models));
  await waitFor(() => expect(trigger.textContent).toContain("Kept model"));
  expect(document.activeElement).toBe(refresh);
  expect(
    (await screen.findByRole("menuitemradio", { name: /Kept model/ })).getAttribute("aria-checked"),
  ).toBe("true");
  expect(gateway.setUtilityRole).not.toHaveBeenCalled();
  expect(read).toHaveBeenCalledTimes(2);
});

it.each(cases)("keeps accepted $name choices during a paused reread", async ({ result }) => {
  const read = vi.fn(async () => result);
  await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("success"));
  read.mockClear();
  pauseQueries();
  act(() => void queryClient.refetchQueries({ queryKey: [MODELS_KEY] }));
  fireEvent.click(screen.getByRole("button", { name: "Utility model" }));
  await screen.findByRole("menuitemradio", { name: "Use Runtime default" });
  expect(screen.queryByText("Loading")).toBeNull();
  expect(screen.queryAllByRole("menuitemradio", { name: /Kept model/ })).toHaveLength(
    result.length,
  );
  expect(queryClient.getQueryData([UTILITY_ROLE_KEY])).toEqual(role);
  expect(read).not.toHaveBeenCalled();
});

it("keeps accepted choices beside a refresh failure", async () => {
  const read = vi
    .fn<() => Promise<SelectableModel[]>>()
    .mockResolvedValueOnce(models)
    .mockRejectedValue(new Error("refresh failed"));
  const gateway = await mount(read);
  await waitFor(() => expect(queryClient.getQueryState([MODELS_KEY])?.status).toBe("success"));
  await act(async () => queryClient.refetchQueries({ queryKey: [MODELS_KEY] }));
  fireEvent.click(screen.getByRole("button", { name: "Utility model" }));
  const option = await screen.findByRole("menuitemradio", { name: /Kept model/ });
  expect(option.getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("menu").textContent).toContain("Couldn't load models.");
  expect(screen.getByRole("menuitem", { name: "Refresh models" })).toBeTruthy();
  expect(gateway.setUtilityRole).not.toHaveBeenCalled();
});
