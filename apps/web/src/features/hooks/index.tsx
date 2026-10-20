import { registerHookDataProviders } from "@/features/hooks/adapters/runtimeDataProviders";
import { type FlameClient } from "@flame/runtime-client";
import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { HOOKS_PANE } from "@/features/settings/public/paneIDs";
import { installHookTrustGateway } from "@/features/hooks/adapters/runtimeHookTrustGateway";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
const HooksPane = lazy(() =>
  import("@/features/hooks/ui/HooksPane").then(({ HooksPane }) => ({ default: HooksPane })),
);

export function createHooksPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.hooks-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      registerHookDataProviders(ctx, runtimeClient);
      let gateway =
        ctx.runtime.connectionGeneration() === null
          ? undefined
          : installHookTrustGateway(runtimeClient);
      ctx.cleanup(() => gateway?.dispose());
      ctx.cleanup(
        followRuntimeGeneration(ctx.runtime, (next) => {
          if (next === null) {
            gateway?.dispose();
            gateway = undefined;
          } else if (gateway) gateway.replaceRuntimeGeneration();
          else gateway = installHookTrustGateway(runtimeClient);
        }),
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
    },
  });
}
