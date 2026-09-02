import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient } from "@/rpc";
import { queryClient } from "@/lib/queryClient";
import {
  setEmbeddingRole as saveEmbeddingRole,
  setUtilityRole as saveUtilityRole,
  updateProvider,
} from "../application/providerConfig";
import {
  EMBEDDING_ROLE_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "../application/providerQueries";
import {
  ProviderConfiguration,
  type ProviderConfigurationSnapshot,
} from "../application/providerModels";
import { installProviderGateway } from "./runtimeProviderGateway";
import { rejected } from "@/test/rejected";

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  resetContainer();
  queryClient.removeQueries({ queryKey: [PROVIDERS_KEY] });
  queryClient.removeQueries({ queryKey: [UTILITY_ROLE_KEY] });
  queryClient.removeQueries({ queryKey: [EMBEDDING_ROLE_KEY] });
});

describe("runtimeProviderGateway", () => {
  it("maps the authoritative provider returned by Runtime", async () => {
    const update = vi.fn().mockResolvedValue({
      id: "openai-compatible",
      baseUrl: "https://models.example.test/v1",
      credential: { masked: "sk****st", source: "stored" },
      configured: true,
      credentialRequirement: "apiKeyRequired",
      requiresBaseUrl: true,
      embeddingCapable: true,
      defaultEmbeddingModel: "embed-1",
    });
    setContainer({
      client: () => ({ providers: { update } }) as unknown as FlameClient,
    });
    uninstall = installProviderGateway().dispose;

    await expect(
      updateProvider({
        provider: "openai-compatible",
        apiKey: { type: "set", value: "sk-test" },
        baseUrl: { type: "set", value: "https://models.example.test/v1" },
      }),
    ).resolves.toEqual(
      ProviderConfiguration.restore({
        id: "openai-compatible",
        baseUrl: "https://models.example.test/v1",
        credential: { masked: "sk****st", source: "stored" },
        configured: true,
        credentialRequirement: "apiKeyRequired",
        requiresBaseUrl: true,
        embeddingCapable: true,
        defaultEmbeddingModel: "embed-1",
      }),
    );
  });

  it("preserves the stored utility and embedding roles", async () => {
    const setUtilityRole = vi.fn().mockResolvedValue({ provider: "openai", model: "chat-1" });
    const setEmbeddingRole = vi.fn().mockResolvedValue({ provider: "openai", model: "embed-1" });
    setContainer({
      client: () => ({ models: { setUtilityRole, setEmbeddingRole } }) as unknown as FlameClient,
    });
    uninstall = installProviderGateway().dispose;

    await expect(saveUtilityRole({ provider: "openai", model: "chat-1" })).resolves.toEqual({
      ok: true,
    });
    await expect(saveEmbeddingRole({ provider: "openai", model: "embed-1" })).resolves.toEqual({
      ok: true,
    });
    expect(queryClient.getQueryData([UTILITY_ROLE_KEY])).toEqual({
      provider: "openai",
      model: "chat-1",
    });
    expect(queryClient.getQueryData([EMBEDDING_ROLE_KEY])).toEqual({
      provider: "openai",
      model: "embed-1",
    });
  });

  it("retires in-flight and queued provider commands before installing a successor", async () => {
    const retiredUpdate = Promise.withResolvers<ProviderConfiguration>();
    const updateRetired = vi.fn(() => retiredUpdate.promise);
    const updateSuccessor = vi.fn().mockResolvedValue({
      id: "openai-compatible",
      configured: true,
      credentialRequirement: "apiKeyRequired",
      requiresBaseUrl: true,
      embeddingCapable: true,
      defaultEmbeddingModel: "embed-1",
      baseUrl: "https://successor.example.test/v1",
      credential: { masked: "successor****key", source: "stored" },
    });
    setContainer({
      client: () => ({ providers: { update: updateRetired } }) as unknown as FlameClient,
    });
    const retiredInstallation = installProviderGateway();
    queryClient.setQueryData([PROVIDERS_KEY], [provider()]);

    const inFlight = updateProvider({
      provider: "openai-compatible",
      baseUrl: { type: "set", value: "https://retired.example.test/v1" },
    });
    const queued = updateProvider({
      provider: "openai-compatible",
      baseUrl: { type: "set", value: "https://queued.example.test/v1" },
    });
    const inFlightSettlement = rejected(inFlight);
    const queuedSettlement = rejected(queued);
    await vi.waitFor(() => expect(updateRetired).toHaveBeenCalledOnce());

    setContainer({
      client: () => ({ providers: { update: updateSuccessor } }) as unknown as FlameClient,
    });
    const successorInstallation = installProviderGateway();
    uninstall = () => {
      successorInstallation.dispose();
      retiredInstallation.dispose();
    };
    queryClient.setQueryData(
      [PROVIDERS_KEY],
      [
        provider({
          baseUrl: "https://successor.example.test/v1",
          credential: { masked: "successor****key", source: "stored" },
        }),
      ],
    );

    retiredUpdate.resolve(
      provider({
        baseUrl: "https://retired.example.test/v1",
        credential: { masked: "retired****key", source: "stored" },
      }),
    );
    await expect(inFlightSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    await expect(queuedSettlement).resolves.toMatchObject({
      message: "provider_mutation_generation_retired",
    });
    expect(updateSuccessor).not.toHaveBeenCalled();
    expect(queryClient.getQueryData([PROVIDERS_KEY])).toEqual([
      provider({
        baseUrl: "https://successor.example.test/v1",
        credential: { masked: "successor****key", source: "stored" },
      }),
    ]);

    const successorCommand = updateProvider({
      provider: "openai-compatible",
      baseUrl: { type: "set", value: "https://successor.example.test/v1" },
    });
    retiredInstallation.replaceRuntimeGeneration();
    await expect(successorCommand).resolves.toMatchObject({
      baseUrl: "https://successor.example.test/v1",
    });
    expect(updateSuccessor).toHaveBeenCalledOnce();
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
