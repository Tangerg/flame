import { getContainer } from "@/main/container";
import { describeProblem, rpcErrorText } from "@/lib/rpcErrors";
import type { FlameClient, Provider, ProviderConfigChange } from "@/rpc";
import type { ProviderGateway, ProviderSettingChange } from "../application/ports/providerGateway";
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
      const result = await client.providers.test(provider);
      return {
        ok: result.ok,
        error: result.ok ? undefined : describeProblem(result.error),
      };
    },
    errorMessage(error) {
      return rpcErrorText(error);
    },
  };
}

function providerConfiguration(provider: Provider): ProviderConfiguration {
  return ProviderConfiguration.restore({
    id: provider.id,
    baseUrl: provider.baseUrl,
    credential: provider.credential,
    configured: provider.configured,
    credentialRequirement: provider.credentialRequirement,
    requiresBaseUrl: provider.requiresBaseUrl,
    embeddingCapable: provider.embeddingCapable,
    defaultEmbeddingModel: provider.defaultEmbeddingModel,
  });
}

function toWireChange(change: ProviderSettingChange | undefined): ProviderConfigChange | undefined {
  if (change === undefined) return undefined;
  return change.type === "clear" ? { type: "clear" } : { type: "set", value: change.value };
}

export function installProviderGateway() {
  const gateway = runtimeProviderGateway(getContainer().client());
  const mutationOwner = ProviderMutationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => mutationOwner.replaceRuntimeGeneration(),
    dispose: () => mutationOwner.dispose(),
  };
}
