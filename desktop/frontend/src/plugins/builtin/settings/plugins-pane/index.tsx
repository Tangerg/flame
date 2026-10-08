import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { asyncDisposeSymbol } from "dougong";
import { lazy } from "react";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/plugins/builtin/runtime/public/services";
import { registerPackageContributions } from "./contributions";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { PLUGINS_PANE } from "../kit/panes";

const PluginsPane = lazy(() =>
  import("./ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export function createPluginsPane(runtimeClient: () => FlameClient, carrier: PluginCarrier) {
  return definePlugin({
    name: "flame.builtin.plugins-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      let current: ContributionLifetime | undefined;
      let retiring = Promise.resolve();
      let latest: object;
      const replace = () => {
        const intent = {};
        latest = intent;
        const predecessor = current;
        current = undefined;
        const release = predecessor?.[asyncDisposeSymbol]() ?? Promise.resolve();
        retiring = retiring
          .then(async () => {
            await release;
            if (
              ctx.signal.aborted ||
              latest !== intent ||
              ctx.runtime.connectionGeneration() === null
            )
              return;
            const owned = ctx.lifetime("runtime-packages");
            try {
              registerPackageContributions(owned, runtimeClient(), carrier);
              current = owned;
            } catch (error) {
              await owned[asyncDisposeSymbol]();
              throw error;
            }
          })
          .catch((error: unknown) => {
            if (!ctx.signal.aborted) ctx.log.error(error);
          });
      };
      ctx.cleanup(followRuntimeGeneration(ctx.runtime, replace));
      ctx.cleanup(() => retiring);
      replace();
      registerSettingsPane(ctx, {
        id: PLUGINS_PANE,
        label: "settings.pane.plugins",
        description: "plugins.hero",
        group: "integrations",
        icon: "blocks",
        order: 99,
        component: PluginsPane,
      });
    },
  });
}
