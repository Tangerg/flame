import { lazy } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { registerProviderDataProviders } from "@/features/providers/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { PROVIDERS_PANE } from "@/features/settings/public/paneIDs";
import { createRuntimeProviderGateway } from "@/features/providers/adapters/runtimeProviderGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
import { type ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
const ProvidersPane = lazy(() =>
  import("@/features/providers/ui/ProvidersPane").then(({ ProvidersPane }) => ({
    default: ProvidersPane,
  })),
);

export function createProvidersPlugin(
  runtimeClient: () => FlameClient,
  mutations: ProviderMutationOwner,
) {
  return definePlugin({
    name: "flame.builtin.providers",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      ctx.cleanup(() => mutations.retireRuntimeGeneration());
      const replace = () =>
        mutations.replaceRuntimeGeneration(() => createRuntimeProviderGateway(runtimeClient()));
      if (ctx.runtime.connectionGeneration() !== null) replace();
      registerProviderDataProviders(ctx, runtimeClient);
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) mutations.retireRuntimeGeneration();
          else replace();
        }),
      );
      registerSettingsPane(ctx, {
        id: PROVIDERS_PANE,
        label: "settings.pane.providers",
        keywords: [
          "providers.apiKey.label",
          "providers.baseUrl.label",
          "providers.utility.title",
          "providers.embedding.title",
        ],
        group: "models",
        icon: "server",
        order: 50,
        component: ProvidersPane,
      });
    },
  });
}
