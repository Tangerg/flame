import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest } from "@/test/kernel";
import { type ProviderGateway } from "@/features/providers/application/ports/providerGateway";
import {
  ProviderConfiguration,
  type ProviderRole,
} from "@/features/providers/application/providerModels";
import { ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
import { SelectableModel } from "@/features/providers/application/selectableModel";
import { EmbeddingModelSection, UtilityModelSection } from "@/features/providers/ui/RoleSections";
let owner: ProviderMutationOwner | undefined;

afterEach(() => {
  cleanup();
  owner?.dispose();
  queryClient.clear();
});

async function installRoles() {
  const state = {
    utility: { provider: "openai", model: "research-one" } as ProviderRole,
    embedding: { provider: "openai", model: "custom-embedding" } as ProviderRole,
  };
  const models = [
    new SelectableModel({ provider: "openai", id: "research-one", label: "Research model" }),
    new SelectableModel({ provider: "anthropic", id: "research-two", label: "Research model" }),
  ];
  const providers = [
    ProviderConfiguration.restore({
      id: "openai",
      configured: true,
      embeddingCapable: true,
      defaultEmbeddingModel: "default-embedding",
    }),
    ProviderConfiguration.restore({ id: "anthropic", configured: true }),
  ];
  const reads = {
    providers: () => providers,
    models: () => models,
    "utility-role": () => state.utility,
    "embedding-role": () => state.embedding,
  };
  await loadPluginsForTest(
    definePlugin({
      name: "test.provider-roles",
      setup(ctx) {
        for (const [key, read] of Object.entries(reads)) {
          ctx.contribute(DATA_PROVIDER, { key, fetcher: async () => read() });
        }
      },
    }),
  );
  const gateway = {
    updateProvider: vi.fn<ProviderGateway["updateProvider"]>(),
    testProvider: vi.fn<ProviderGateway["testProvider"]>(),
    setUtilityRole: vi.fn<ProviderGateway["setUtilityRole"]>(async (role) => {
      state.utility = role;
      return role;
    }),
    setEmbeddingRole: vi.fn<ProviderGateway["setEmbeddingRole"]>(async (role) => {
      state.embedding = role;
      return role;
    }),
    errorMessage: (error: unknown) => (error instanceof Error ? error.message : undefined),
  } satisfies ProviderGateway;
  owner = testClient.providers.mutations;
  owner.replaceRuntimeGeneration(() => gateway);
  return { state, gateway };
}

it("distinguishes identical names and waits for the accepted provider/model pair", async () => {
  const { state, gateway } = await installRoles();
  const accepted = Promise.withResolvers<ProviderRole>();
  gateway.setUtilityRole.mockImplementation(async () => {
    const saved = await accepted.promise;
    state.utility = saved;
    return saved;
  });
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <UtilityModelSection />
    </QueryClientProvider>,
  );
  const trigger = await screen.findByRole("button", {
    name: "Utility model",
    description: /openai \/ research-one/,
  });
  const value = document.getElementById(trigger.getAttribute("aria-describedby")!);
  expect(value?.textContent).toContain("openai / research-one");
  fireEvent.click(trigger);
  const previous = await screen.findByRole("menuitemradio", { name: /openai \/ research-one/ });
  const next = screen.getByRole("menuitemradio", { name: /anthropic \/ research-two/ });
  expect(previous.getAttribute("aria-checked")).toBe("true");
  expect(next.getAttribute("aria-checked")).toBe("false");
  fireEvent.click(next);
  await waitFor(() => expect(gateway.setUtilityRole).toHaveBeenCalledOnce());
  expect(gateway.setUtilityRole).toHaveBeenCalledWith({
    provider: "anthropic",
    model: "research-two",
  });
  expect(trigger.getAttribute("aria-disabled")).toBe("true");
  expect(queryClient.getQueryData(["utility-role"])).toEqual({
    provider: "openai",
    model: "research-one",
  });
  await act(async () => accepted.resolve({ provider: "anthropic", model: "research-two" }));
  await waitFor(() => expect(value?.textContent).toContain("anthropic / research-two"));
  fireEvent.click(trigger);
  expect(
    (await screen.findByRole("menuitemradio", { name: /anthropic \/ research-two/ })).getAttribute(
      "aria-checked",
    ),
  ).toBe("true");
  expect(gateway.setUtilityRole).toHaveBeenCalledOnce();
});

it("keeps a custom embedding role distinct from its provider default after a rejected selection", async () => {
  const { gateway } = await installRoles();
  gateway.setEmbeddingRole.mockRejectedValue(new Error("The role update was rejected."));
  testClient.render(
    <QueryClientProvider client={queryClient}>
      <EmbeddingModelSection />
    </QueryClientProvider>,
  );
  const trigger = await screen.findByRole("button", {
    name: "Embedding model",
    description: /custom-embedding/,
  });
  fireEvent.click(trigger);
  const preset = await screen.findByRole("menuitemradio", { name: /default-embedding/ });
  const off = screen.getByRole("menuitemradio", { name: "Off" });
  expect(preset.getAttribute("aria-checked")).toBe("false");
  expect(off.getAttribute("aria-checked")).toBe("false");
  fireEvent.click(preset);
  await screen.findByText("The role update was rejected.");
  expect(gateway.setEmbeddingRole).toHaveBeenCalledWith({
    provider: "openai",
    model: "default-embedding",
  });
  expect(queryClient.getQueryData(["embedding-role"])).toEqual({
    provider: "openai",
    model: "custom-embedding",
  });
  expect(trigger.textContent).toContain("custom-embedding");
  fireEvent.click(trigger);
  expect(
    (await screen.findByRole("menuitemradio", { name: /default-embedding/ })).getAttribute(
      "aria-checked",
    ),
  ).toBe("false");
});
