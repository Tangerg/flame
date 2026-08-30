import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ShortcutsProvider } from "@/plugins/host/ShortcutsProvider";
import { ShortcutsPane } from "./ShortcutsPane";

export default definePlugin({
  name: "flame.builtin.shortcuts",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "shortcuts-provider",
      order: 50,
      component: ShortcutsProvider,
    });

    ctx.contribute(SETTINGS_PANE, {
      id: "shortcuts",
      label: "settings.pane.shortcuts",
      description: "shortcuts.sub",
      group: "general",
      icon: "command",
      order: 10,
      component: ShortcutsPane,
    });
  },
});
