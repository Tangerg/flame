import { type InstalledPluginsOwner } from "@/features/plugins/adapters/installedPlugins";
import { lazy } from "react";
import { RUNTIME_STREAM } from "@/features/connection/public/services";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { PLUGINS_PANE } from "@/features/settings/public/paneIDs";
const PluginsPane = lazy(() =>
  import("@/features/plugins/ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export function createPluginsPlugin(installed: InstalledPluginsOwner) {
  return definePlugin({
    name: "flame.builtin.plugins-pane",
    requires: { runtime: RUNTIME_STREAM },
    setup(ctx) {
      installed.start(ctx, ctx.runtime);
      ctx.cleanup(installed.dispose);
      registerSettingsPane(ctx, {
        id: PLUGINS_PANE,
        label: "settings.pane.plugins",
        keywords: [
          "packages.installTitle",
          "packages.installedTitle",
          "packages.providers.title",
          "plugins.loadedTitle",
        ],
        group: "integrations",
        icon: "blocks",
        order: 99,
        component: PluginsPane,
      });
    },
  });
}
