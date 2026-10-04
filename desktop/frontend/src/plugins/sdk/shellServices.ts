import { definePlugin } from "dougong";
import { getConfig, hasConfig, setConfig, useConfigStore } from "./config";
import { executeCommand } from "./selectors/commands";
import {
  COMMANDS,
  CONFIG,
  WINDOW,
  type CommandsService,
  type ConfigService,
  type WindowService,
} from "./services";
import { useWindowStore } from "./windowStore";

const config: ConfigService = {
  get: (key, defaultValue) => getConfig(key, defaultValue),
  set: (key, value) => setConfig(key, value),
  has: (key) => hasConfig(key),
  onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
};

const window: WindowService = {
  setTitle: (text) => useWindowStore.getState().setTitle(text),
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
