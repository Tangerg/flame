import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { useCommandMenuStore } from "@/features/command/menu/application/commandMenuState";
import { COMMAND_MENU_COMMAND } from "@/features/command/menu/public/commandMenu";
import { CommandMenu } from "@/features/command/menu/ui/CommandMenu";
export default definePlugin({
  name: "flame.builtin.command-menu",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "command-menu",
      order: 20,
      component: CommandMenu,
    });
    ctx.contribute(COMMAND, {
      id: COMMAND_MENU_COMMAND,
      label: "command.openCommandMenu",
      combo: "Mod+Shift+P",
      run: () => useCommandMenuStore.getState().toggle(),
    });
  },
});
