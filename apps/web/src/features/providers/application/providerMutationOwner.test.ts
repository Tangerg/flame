import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
import { type ProviderGateway } from "@/features/providers/application/ports/providerGateway";
import { MODELS_KEY, PROVIDERS_KEY } from "@/features/providers/application/providerQueries";
import {
  ProviderConfiguration,
  type ProviderConfigurationSnapshot,
} from "@/features/providers/application/providerModels";
import { rejected } from "@/test/rejected";
let owner: ProviderMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
  queryClient.removeQueries({ queryKey: [MODELS_KEY] });
  vi.restoreAllMocks();
});

describe("ProviderMutationOwner", () => {
  it("rejects a captured command instead of retargeting it to a successor", async () => {
    const first = vi.fn(async () => provider());
    const successor = vi.fn(async () => provider());
    owner = new ProviderMutationOwner({ updateProvider: first } as unknown as ProviderGateway);
    const commands = owner.capture()!;
    const captured = commands.updateProvider.bind(commands);
    owner.replaceRuntimeGeneration(
      () => ({ updateProvider: successor }) as unknown as ProviderGateway,
    );
    await expect(captured({ provider: "openai" })).rejects.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    expect(first).not.toHaveBeenCalled();
    expect(successor).not.toHaveBeenCalled();
  });

  it("publishes one instance's material generation for connection, replacement and disposal", () => {
    owner = new ProviderMutationOwner(null);
    const start = owner.materialGeneration();
    owner.replaceRuntimeGeneration(() => ({}) as ProviderGateway);
    expect(owner.materialGeneration()).toBe(start + 1n);
    owner.replaceRuntimeGeneration(() => ({}) as ProviderGateway);
    expect(owner.materialGeneration()).toBe(start + 2n);
    owner.dispose();
    expect(owner.materialGeneration()).toBe(start + 3n);
    owner = undefined;
  });

  it("retires one Runtime generation without settling its commands into the successor", async () => {
    const retired = Promise.withResolvers<ProviderConfiguration>();
    const updateProvider = vi
      .fn()
      .mockReturnValueOnce(retired.promise)
      .mockResolvedValueOnce(provider({ baseUrl: "https://successor.example.test/v1" }));
    owner = new ProviderMutationOwner({ updateProvider } as unknown as ProviderGateway);
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const inFlight = owner.capture()!.updateProvider({
      provider: "openai-compatible",
      baseUrl: { type: "set", value: "https://retired.example.test/v1" },
    });
    const queued = owner.capture()!.updateProvider({
      provider: "openai-compatible",
      baseUrl: { type: "set", value: "https://queued.example.test/v1" },
    });
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateProvider).toHaveBeenCalledOnce());

    owner.replaceRuntimeGeneration(() => ({ updateProvider }) as unknown as ProviderGateway);
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    expect(updateProvider).toHaveBeenCalledOnce();

    retired.resolve(provider({ baseUrl: "https://retired.example.test/v1" }));
    await Promise.resolve();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([provider()]);

    await expect(
      owner.capture()!.updateProvider({
        provider: "openai-compatible",
        baseUrl: { type: "set", value: "https://successor.example.test/v1" },
      }),
    ).resolves.toEqual(provider({ baseUrl: "https://successor.example.test/v1" }));
    expect(updateProvider).toHaveBeenCalledTimes(2);
  });

  it("does not globally serialize unrelated provider resources", async () => {
    const first = Promise.withResolvers<ProviderConfiguration>();
    const updateProvider = vi.fn((input: { provider: string }) =>
      input.provider === "openai"
        ? first.promise
        : Promise.resolve(provider({ id: input.provider, baseUrl: "https://independent.test" })),
    );
    owner = new ProviderMutationOwner({ updateProvider } as unknown as ProviderGateway);

    const blocked = owner.capture()!.updateProvider({
      provider: "openai",
      baseUrl: { type: "set", value: "https://blocked.test" },
    });
    const independent = owner.capture()!.updateProvider({
      provider: "deepseek",
      baseUrl: { type: "set", value: "https://independent.test" },
    });

    await vi.waitFor(() => expect(updateProvider).toHaveBeenCalledTimes(2));
    await expect(independent).resolves.toMatchObject({ id: "deepseek" });
    first.resolve(provider({ id: "openai", baseUrl: "https://blocked.test" }));
    await expect(blocked).resolves.toMatchObject({ id: "openai" });
  });

  it("does not turn failed cache repair into an accepted provider command failure", async () => {
    const saved = provider({ baseUrl: "https://saved.example.test/v1" });
    owner = new ProviderMutationOwner({
      updateProvider: vi.fn().mockResolvedValue(saved),
    } as unknown as ProviderGateway);
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("read unavailable"));

    await expect(
      owner.capture()!.updateProvider({
        provider: saved.id,
        baseUrl: { type: "set", value: saved.baseUrl! },
      }),
    ).resolves.toEqual(saved);
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([saved]);
  });

  it("retires a live provider probe without publishing its late result", async () => {
    const probe = Promise.withResolvers<{ ok: boolean }>();
    owner = new ProviderMutationOwner({
      testProvider: vi.fn(() => probe.promise),
    } as unknown as ProviderGateway);

    const result = rejected(owner.capture()!.testProvider("openai"));
    owner.replaceRuntimeGeneration(() => ({}) as ProviderGateway);
    await expect(result).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    probe.resolve({ ok: true });
  });
});

function provider(overrides: Partial<ProviderConfigurationSnapshot> = {}): ProviderConfiguration {
  return ProviderConfiguration.restore({
    id: "openai-compatible",
    configured: false,
    requiresBaseUrl: true,
    embeddingCapable: true,
    defaultEmbeddingModel: "embed-1",
    ...overrides,
  });
}
