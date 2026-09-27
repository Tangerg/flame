import { registerHookDataProviders } from "./adapters/runtimeDataProviders";
import type { FlameClient } from "@flame/runtime-contract/client";
import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { HOOKS_PANE } from "../kit/panes";
import { installHookTrustGateway } from "./adapters/runtimeHookTrustGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";

const HooksPane = lazy(() =>
  import("./ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export function createHooksPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.hooks-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      registerHookDataProviders(ctx, runtimeClient);
      const gateway = installHookTrustGateway(runtimeClient);
      const unsubscribeRuntime = followRuntimeGeneration(ctx.runtime, () =>
        gateway.replaceRuntimeGeneration(),
      );
      registerSettingsPane(ctx, {
        id: HOOKS_PANE,
        label: "settings.pane.hooks",
        description: "hooks.intro",
        group: "agent",
        icon: "lightning",
        order: 57,
        component: HooksPane,
      });
      ctx.cleanup(() => {
        unsubscribeRuntime();
        gateway.dispose();
      });
    },
  });
}
