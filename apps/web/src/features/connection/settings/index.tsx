import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { registerSettingsPane } from "@/features/settings/public/registerSettingsPane";
import { CONNECTION_PANE } from "@/features/settings/public/paneIDs";
const ConnectionPane = lazy(() =>
  import("@/features/connection/settings/ui/ConnectionPane").then(({ ConnectionPane }) => ({
    default: ConnectionPane,
  })),
);

export default definePlugin({
  name: "flame.builtin.connection-settings",
  setup(ctx) {
    registerSettingsPane(ctx, {
      id: CONNECTION_PANE,
      label: "settings.pane.connection",
      keywords: ["settings.connection.title", "settings.connection.url"],
      group: "general",
      icon: "network",
      order: 5,
      component: ConnectionPane,
    });
  },
});
