import { PRODUCT_NAME } from "@/app/product";
import { definePlugin } from "dougong";
import { CONFIG, Configuration } from "@/extensions/config";
import { executeCommand, COMMANDS } from "@/features/command/public/commands";
import { WINDOW, WindowTitle } from "@/features/shell/public/window";

export const shellServices = definePlugin({
  name: "flame.kernel.shell",
  provides: {
    config: CONFIG,
    window: WINDOW,
    commands: COMMANDS,
  },
  setup: () => ({
    config: new Configuration(),
    window: new WindowTitle(PRODUCT_NAME),
    commands: { execute: executeCommand },
  }),
});
