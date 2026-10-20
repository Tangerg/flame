import { PRODUCT_NAME } from "@/app/product";
import { definePlugin } from "dougong";
import { getConfig, hasConfig, setConfig, useConfigStore } from "@/extensions/config";
import { executeCommand } from "@/features/command/public/commands";
import { COMMANDS, type CommandsService } from "@/features/command/public/commands";
import { CONFIG, type ConfigService } from "@/extensions/config";
import { WINDOW, type WindowService } from "@/features/shell/public/window";
import { useWindowStore } from "@/features/shell/public/windowStore";
const config: ConfigService = {
  get: (key, defaultValue) => getConfig(key, defaultValue),
  set: (key, value) => setConfig(key, value),
  has: (key) => hasConfig(key),
  onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
};

const window: WindowService = {
  setTitle: (text) => useWindowStore.getState().setTitle(text || PRODUCT_NAME),
  setBadge: (n) => useWindowStore.getState().setBadge(Math.max(0, n ?? 0)),
  setWorking: (on) => useWindowStore.getState().setWorking(on),
};

const commands: CommandsService = {
  execute: (id, ...args) => executeCommand(id, ...args),
};

export const shellServices = definePlugin({
  name: "flame.kernel.shell",
  provides: {
    config: CONFIG,
    window: WINDOW,
    commands: COMMANDS,
  },
  setup: () => ({ config, window, commands }),
});
