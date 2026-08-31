import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { registerSettingsPane } from "../kit";
import { PLUGINS_PANE } from "../kit/panes";

const PluginsPane = lazy(() =>
  import("./ui/PluginsPane").then(({ PluginsPane }) => ({ default: PluginsPane })),
);

export default definePlugin({
  name: "flame.builtin.plugins-pane",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: PLUGINS_PANE,
      label: "settings.pane.plugins",
      group: "integrations",
      icon: "tool",
      order: 99,
      component: PluginsPane,
    });
  },
});
