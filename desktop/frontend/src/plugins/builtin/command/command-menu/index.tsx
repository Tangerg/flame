import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMMAND } from "@/plugins/sdk/kernelPoints";
import { useCommandMenuStore } from "./application/commandMenuState";
import { COMMAND_MENU_COMMAND } from "./public/commandMenu";
import { CommandMenu } from "./ui/CommandMenu";

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
