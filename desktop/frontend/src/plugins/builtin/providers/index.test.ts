import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { updateProvider } from "./application/providerConfig";
import { PROVIDERS_KEY } from "./application/providerQueries";
import {
  ProviderConfiguration,
  type ProviderConfigurationSnapshot,
} from "./application/providerModels";
import { createProvidersPlugin } from ".";
import { rejected } from "@/test/rejected";
import { startKernel, stopKernel } from "@/plugins/sdk/bootstrap";
import { ProviderMutationOwner } from "./application/providerMutationOwner";

afterEach(async () => {
  await resetKernelForTest();
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
});

describe("providers plugin Runtime generation wiring", () => {
  it("withdraws its gateway when a later setup step fails", async () => {
    const runtime = definePlugin({
      name: "test.failed-subscription",
      provides: { stream: RUNTIME_STREAM },
      setup: () => ({
        stream: {
          connectionGeneration: () => null,
          subscribeConnection() {
            throw new Error("subscription failed");
          },
          reportConnectionLoss() {},
        },
      }),
    });

    await expect(
      startKernel([runtime, createProvidersPlugin(() => ({}) as FlameClient)]),
    ).rejects.toThrow("subscription failed");

    expect(() => ProviderMutationOwner.current()).toThrow("not installed");
  });

  it("withdraws its gateway even when the subscription teardown fails", async () => {
    const unsubscribe = vi.fn(() => {
      throw new Error("subscription teardown failed");
    });
    const runtime = definePlugin({
      name: "test.failed-unsubscribe",
      provides: { stream: RUNTIME_STREAM },
      setup: () => ({
        stream: {
          connectionGeneration: () => null,
          subscribeConnection: () => unsubscribe,
          reportConnectionLoss() {},
        },
      }),
    });
    const host = await startKernel([runtime, createProvidersPlugin(() => ({}) as FlameClient)]);

    await expect(stopKernel(host)).rejects.toThrow(/dispos|stop|teardown/i);

    expect(unsubscribe).toHaveBeenCalledOnce();
    expect(() => ProviderMutationOwner.current()).toThrow("not installed");
  });

  it("retires an admitted command when the Runtime process generation changes", async () => {
    const retired = Promise.withResolvers<ProviderConfigurationSnapshot>();
    const update = vi.fn(() => retired.promise);
    const updateSuccessor = vi.fn().mockResolvedValue({
      id: "openai-compatible",
      configured: false,
      credentialRequirement: "apiKeyRequired",
      baseUrl: "https://successor.example.test/v1",
    });
    let activeClient = { providers: { update } } as unknown as FlameClient;
    const runtimeClient = () => activeClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, createProvidersPlugin(runtimeClient));
    expect(lookupDataProvider(PROVIDERS_KEY)).toBeDefined();
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const command = rejected(
      updateProvider({
        provider: "openai-compatible",
        baseUrl: { type: "set", value: "https://retired.example.test/v1" },
      }),
    );
    await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());

    activeClient = { providers: { update: updateSuccessor } } as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });

    retired.resolve({
      id: "openai-compatible",
      configured: false,
      credentialRequirement: "apiKeyRequired",
      baseUrl: "https://retired.example.test/v1",
    });
    await Promise.resolve();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([provider()]);

    await expect(
      updateProvider({
        provider: "openai-compatible",
        baseUrl: { type: "set", value: "https://successor.example.test/v1" },
      }),
    ).resolves.toMatchObject({ baseUrl: "https://successor.example.test/v1" });
    expect(update).toHaveBeenCalledOnce();
    expect(updateSuccessor).toHaveBeenCalledOnce();
    await resetKernelForTest();
    expect(lookupDataProvider(PROVIDERS_KEY)).toBeUndefined();
  });
});

function provider(overrides: Partial<ProviderConfigurationSnapshot> = {}): ProviderConfiguration {
  return ProviderConfiguration.restore({
    id: "openai-compatible",
    configured: false,
    credentialRequirement: "apiKeyRequired",
    requiresBaseUrl: true,
    embeddingCapable: true,
    defaultEmbeddingModel: "embed-1",
    ...overrides,
  });
}
