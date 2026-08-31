// The plugin that provides the shell's Services.
//
// One plugin rather than four: the reason any of these changes is the same one —
// the app shell grew a capability — and splitting a tiny provider per
// contract buys indirection, not cohesion. The consumers are still decoupled;
// each declares only the contract it uses, which is the property that matters.
//
// WORKSPACE is the exception, and provided by the workspace context instead: its
// state is a context aggregate, and reaching one from here would invert the
// platform's direction.

import { definePlugin } from "dougong";
import { addLocaleBundle } from "@/lib/i18n";
import { getConfig, hasConfig, setConfig, useConfigStore } from "./config";
import { executeCommand } from "./selectors/commands";
import {
  COMMANDS,
  CONFIG,
  I18N,
  WINDOW,
  type CommandsService,
  type ConfigService,
  type I18nService,
  type WindowService,
} from "./services";
import { useWindowStore } from "./windowStore";

const config: ConfigService = {
  get: (key, defaultValue) => getConfig(key, defaultValue),
  set: (key, value) => setConfig(key, value),
  has: (key) => hasConfig(key),
  onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
};

const i18n: I18nService = {
  // i18next has no per-key removal, so a bundle is permanent for the session.
  // Safe: `t()` only matters while the contributing plugin's UI is mounted, and a
  // same-name reload overwrites the same keys.
  addBundle: (locale, dict) => addLocaleBundle(locale, dict),
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
    i18n: I18N,
    window: WINDOW,
    commands: COMMANDS,
  },
  setup: () => ({ config, i18n, window, commands }),
});
