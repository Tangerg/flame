import type { Contributor } from "@/plugins/sdk";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import type { FlameClient } from "@flame/runtime-contract/client";
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "../application/providerQueries";
import { ProviderConfiguration } from "../application/providerModels";
import { SelectableModel } from "../application/selectableModel";

function pageData<T>(request: Promise<{ data: T[] }>): Promise<T[]> {
  return request.then((page) => page.data);
}

export function registerProviderDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.contribute(DATA_PROVIDER, {
    key: MODELS_KEY,
    fetcher: async (_params, signal) => {
      const client = runtimeClient();
      const configured = (await pageData(client.providers.list(signal))).filter(
        (provider) => provider.configured,
      );
      const lists = await Promise.all(
        configured.map((provider) => pageData(client.models.list(provider.id, signal))),
      );
      return lists.flat().map(
        (m) =>
          new SelectableModel({
            id: m.id,
            provider: m.provider,
            label: m.displayName ?? m.id,
            tokenLimits: m.tokenLimits,
            knowledgeCutoff: m.knowledgeCutoff,
            deprecated: m.deprecated,
            reasoning: m.capabilities?.reasoning,
            reasoningLevels: m.capabilities?.reasoningLevels,
            reasoningDefaultLevel: m.capabilities?.reasoningDefaultLevel,
            inputModalities: m.capabilities?.inputModalities,
            outputModalities: m.capabilities?.outputModalities,
            toolUse: m.capabilities?.toolUse,
            structuredOutput: m.capabilities?.structuredOutput,
          }),
      );
    },
  });
  ctx.contribute(DATA_PROVIDER, {
    key: PROVIDERS_KEY,
    fetcher: async (_params, signal) =>
      (await pageData(runtimeClient().providers.list(signal))).map((provider) =>
        ProviderConfiguration.restore(provider),
      ),
  });
  ctx.contribute(DATA_PROVIDER, {
    key: UTILITY_ROLE_KEY,
    fetcher: (_params, signal) => runtimeClient().models.getUtilityRole(signal),
  });
  ctx.contribute(DATA_PROVIDER, {
    key: EMBEDDING_ROLE_KEY,
    fetcher: (_params, signal) => runtimeClient().models.getEmbeddingRole(signal),
  });
}
