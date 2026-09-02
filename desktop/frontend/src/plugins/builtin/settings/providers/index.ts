import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { PROVIDERS_PANE } from "../kit/panes";
import { installProviderGateway } from "./adapters/runtimeProviderGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

const ProvidersPane = lazy(() =>
  import("./ui/ProvidersPane").then(({ ProvidersPane }) => ({ default: ProvidersPane })),
);

export default definePlugin({
  name: "flame.builtin.providers-pane",
  requires: { runtime: RUNTIME_STREAM },
  setup(ctx) {
    const gateway = installProviderGateway();
    const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
      gateway.replaceRuntimeGeneration(),
    );
    registerSettingsPane(ctx, {
      id: PROVIDERS_PANE,
      label: "settings.pane.providers",
      group: "models",
      icon: "spark",
      order: 50,
      component: ProvidersPane,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
