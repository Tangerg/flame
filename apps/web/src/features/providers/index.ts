import { lazy } from "react";
import { observeProviderDraftLifetime } from "@/features/providers/application/providerDrafts";
import { type FlameClient } from "@flame/runtime-client";
import { registerProviderDataProviders } from "@/features/providers/adapters/runtimeDataProviders";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { PROVIDERS_PANE } from "@/features/settings/public/paneIDs";
import { installProviderGateway } from "@/features/providers/adapters/runtimeProviderGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
const ProvidersPane = lazy(() =>
  import("@/features/providers/ui/ProvidersPane").then(({ ProvidersPane }) => ({
    default: ProvidersPane,
  })),
);

export function createProvidersPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.providers",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      let gateway =
        ctx.runtime.connectionGeneration() === null
          ? undefined
          : installProviderGateway(runtimeClient);
      ctx.cleanup(() => gateway?.dispose());
      ctx.cleanup(observeProviderDraftLifetime());
      registerProviderDataProviders(ctx, runtimeClient);
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) {
            gateway?.dispose();
            gateway = undefined;
          } else if (gateway) gateway.replaceRuntimeGeneration();
          else gateway = installProviderGateway(runtimeClient);
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
