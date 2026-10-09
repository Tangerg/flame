import { lazy } from "react";
import { observeProviderDraftLifetime } from "./application/providerDrafts";
import type { FlameClient } from "@flame/runtime-contract/client";
import { registerProviderDataProviders } from "./adapters/runtimeDataProviders";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../settings/kit";
import { PROVIDERS_PANE } from "../settings/kit/panes";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

const ProvidersPane = lazy(() =>
  import("./ui/ProvidersPane").then(({ ProvidersPane }) => ({ default: ProvidersPane })),
);

export function createProvidersPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.providers",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const gateway = installProviderGateway(runtimeClient);
      ctx.cleanup(() => gateway.dispose());
      ctx.cleanup(observeProviderDraftLifetime());
      registerProviderDataProviders(ctx, runtimeClient);
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, () => gateway.replaceRuntimeGeneration()));
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
