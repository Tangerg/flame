import { asyncDisposeSymbol } from "dougong";
import { lazy } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { RUNTIME_STREAM, followRuntimeGeneration } from "@/features/connection/public/services";
import { registerPackageContributions } from "@/features/plugins/contributions";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { PLUGINS_PANE } from "@/features/settings/public/paneIDs";
const PluginsPane = lazy(() =>
  import("@/features/plugins/ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export function createPluginsPlugin(
  runtimeClient: () => FlameClient,
  runtimeEndpoint: () => string,
) {
  return definePlugin({
    name: "flame.builtin.plugins-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      const modules = new ComponentModules();
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
              registerPackageContributions(
                owned,
                runtimeClient(),
                modules,
                ctx.runtime.connectionGeneration()!,
                runtimeEndpoint(),
              );
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
