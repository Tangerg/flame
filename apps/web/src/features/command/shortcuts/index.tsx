import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { ShortcutsProvider } from "@/features/command/ShortcutsProvider";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { ShortcutsPane } from "@/features/command/shortcuts/ShortcutsPane";
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
      icon: "keyboard",
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
