import { rpcErrorText } from "@/lib/rpcErrors";
import { t } from "@/lib/i18n";
import type { FlameClient, Provider, ProviderConfigChange } from "@flame/runtime-contract/client";
import type { ProviderTestResult } from "@flame/runtime-contract/wire";
import type {
  ProviderGateway,
  ProviderSettingChange,
  ProviderTestOutcome,
} from "../application/ports/providerGateway";
import { ProviderConfiguration } from "../application/providerModels";
import { ProviderMutationOwner } from "../application/providerMutationOwner";

function runtimeProviderGateway(client: FlameClient): ProviderGateway {
  return {
    async updateProvider(input) {
      const saved = await client.providers.update({
        provider: input.provider,
        apiKey: toWireChange(input.apiKey),
        baseUrl: toWireChange(input.baseUrl),
      });
      return providerConfiguration(saved);
    },
    async setUtilityRole(role) {
      const saved = await client.models.setUtilityRole(role);
      return { provider: saved.provider, model: saved.model };
    },
    async setEmbeddingRole(role) {
      const saved = await client.models.setEmbeddingRole(role);
      return { provider: saved.provider, model: saved.model };
    },
    async testProvider(provider) {
      return testOutcome(await client.providers.test(provider));
    },
    errorMessage(error) {
      return rpcErrorText(error);
    },
  };
}

function testOutcome(result: ProviderTestResult): ProviderTestOutcome {
  switch (result.outcome) {
    case "reachable":
      return { ok: true };
    case "notConfigured":
      return { ok: false, error: t("providers.testOutcome.notConfigured") };
    case "invalidCredentials":
      return { ok: false, error: t("rpcError.invalid_api_key") };
    case "timedOut":
      return { ok: false, error: t("rpcError.timeout") };
    case "failed":
      return { ok: false, error: t("providers.testOutcome.failed") };
    default:
      throw new Error(
        `runtime contract violation: provider test outcome ${String((result as { outcome: unknown }).outcome)}`,
      );
  }
}

function providerConfiguration(provider: Provider): ProviderConfiguration {
  return ProviderConfiguration.restore({
    id: provider.id,
    baseUrl: provider.baseUrl,
    credential: provider.credential,
    configured: provider.configured,
    requiresBaseUrl: provider.requiresBaseUrl,
    embeddingCapable: provider.embeddingCapable,
    defaultEmbeddingModel: provider.defaultEmbeddingModel,
  });
}

function toWireChange(change: ProviderSettingChange | undefined): ProviderConfigChange | undefined {
  if (change === undefined) return undefined;
  return change.type === "clear" ? { type: "clear" } : { type: "set", value: change.value };
}

export function installProviderGateway(runtimeClient: () => FlameClient) {
  const gateway = runtimeProviderGateway(runtimeClient());
  const mutationOwner = ProviderMutationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () =>
      mutationOwner.replaceRuntimeGeneration(() => runtimeProviderGateway(runtimeClient())),
    dispose: () => mutationOwner.dispose(),
  };
}
