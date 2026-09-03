import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMMAND, SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { ShortcutsProvider } from "@/plugins/host/ShortcutsProvider";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { ShortcutsPane } from "./ShortcutsPane";

const SHORTCUTS_PANE = "shortcuts";

export default definePlugin({
  name: "flame.builtin.shortcuts",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "shortcuts-provider",
      order: 50,
      component: ShortcutsProvider,
    });

    ctx.contribute(SETTINGS_PANE, {
      id: SHORTCUTS_PANE,
      label: "settings.pane.shortcuts",
      description: "shortcuts.sub",
      group: "general",
      icon: "command",
      order: 10,
      component: ShortcutsPane,
    });

    ctx.contribute(COMMAND, {
      id: "shortcuts.show",
      label: "command.showShortcuts",
      combo: "Mod+/",
      run: () => openWorkspaceSettingsPane(SHORTCUTS_PANE),
    });
  },
});
