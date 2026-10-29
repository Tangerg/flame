import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
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
} from "@/features/providers/application/providerQueries";
import { ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
import { type ProviderGateway } from "@/features/providers/application/ports/providerGateway";
import { ProvidersPane } from "@/features/providers/ui/ProvidersPane";

let owner: ProviderMutationOwner | undefined;
afterEach(() => {
  cleanup();
  owner?.dispose();
  owner = undefined;
  queryClient.clear();
});

const provider = ProviderConfiguration.restore({ id: "openai", configured: true });
const cases = [
  { name: "empty", providers: [] as ProviderConfiguration[], visibleText: "No providers" },
  { name: "nonempty", providers: [provider], visibleText: "openai" },
];

async function mount(providers: ProviderConfiguration[]) {
  const list = vi.fn(async () => providers);
  await contributeForTest((ctx) => {
    ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: list });
    ctx.contribute(DATA_PROVIDER, { key: MODELS_KEY, fetcher: async () => [] });
    for (const key of [UTILITY_ROLE_KEY, EMBEDDING_ROLE_KEY])
      ctx.contribute(DATA_PROVIDER, { key, fetcher: async () => ({}) });
  });
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
      <ProvidersPane />
      <textarea aria-label="Later draft" defaultValue="Keep this provider draft." />
    </>,
  );
  return { list, gateway };
}

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

it.each(cases)(
  "waits for the first paused $name provider result",
  async ({ providers, visibleText }) => {
    pauseQueries();
    const { list, gateway } = await mount(providers);
    await waitFor(() =>
      expect(queryClient.getQueryState([PROVIDERS_KEY])).toMatchObject({
        status: "pending",
        fetchStatus: "paused",
        data: undefined,
      }),
    );
    expect(list).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("Loading");
    expect(screen.queryByText("No providers")).toBeNull();
    expect(screen.getByRole("button", { name: "Utility model" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(screen.getByRole("button", { name: "Embedding model" }).hasAttribute("disabled")).toBe(
      true,
    );
    const draft = screen.getByRole("textbox", { name: "Later draft" });
    act(() => draft.focus());

    await act(async () => onlineManager.setOnline(true));
    expect(await screen.findByText(visibleText)).toBeTruthy();
    expect(document.activeElement).toBe(draft);
    expect(draft).toHaveProperty("value", "Keep this provider draft.");
    expect(gateway.updateProvider).not.toHaveBeenCalled();
    expect(gateway.testProvider).not.toHaveBeenCalled();
    expect(gateway.setUtilityRole).not.toHaveBeenCalled();
    expect(gateway.setEmbeddingRole).not.toHaveBeenCalled();
  },
);

it.each(cases)(
  "keeps accepted $name providers during a paused reread",
  async ({ providers, visibleText }) => {
    const { list } = await mount(providers);
    await screen.findByText(visibleText);
    list.mockClear();
    pauseQueries();
    act(() => void queryClient.refetchQueries({ queryKey: [PROVIDERS_KEY] }));
    await waitFor(() =>
      expect(queryClient.getQueryState([PROVIDERS_KEY])).toMatchObject({
        status: "success",
        fetchStatus: "paused",
        data: providers,
      }),
    );
    expect(list).not.toHaveBeenCalled();
    expect(screen.getByText(visibleText)).toBeTruthy();
  },
);
