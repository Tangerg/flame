// What a plugin declares in `requires`. Anything that is really "a point plus a call to
// `ctx.contribute`" does NOT belong here.

import { service } from "dougong";
import type { ConfigValue } from "./config";
import type { KeyValueStore } from "./storage";
import type { Disposable } from "./types/common";
import type { NotificationLevel, TaskHandle, TaskStartOptions } from "./types/infra";

export interface ConfigService {
  get(key: string, defaultValue?: ConfigValue): ConfigValue | undefined;
  set(key: string, value: ConfigValue): void;
  /** True for any present value, regardless of falsiness. */
  has(key: string): boolean;
  /** Receives the new value, or `undefined` when the key is cleared. */
  onChange(key: string, fn: (value: ConfigValue | undefined) => void): Disposable;
}

export interface I18nService {
  /** Plugin keys live ALONGSIDE the kernel's; last writer wins on collision. */
  addBundle(locale: string, dict: Record<string, string>): void;
}

export interface WindowService {
  /** Latest setter wins. */
  setTitle(text: string): void;
  /** `0` or `undefined` clears it. */
  setBadge(n?: number): void;
  setWorking(on: boolean): void;
}

export interface WorkspaceService {
  /** Opens or focuses. */
  openView(id: string): void;
  closeView(id: string): void;
}

export interface CommandsService {
  /** Warns and no-ops on an unknown id. */
  execute(id: string, ...args: unknown[]): Promise<void>;
}

export const CONFIG = service<ConfigService>("flame.shell.config");
export const I18N = service<I18nService>("flame.shell.i18n");
export const WINDOW = service<WindowService>("flame.shell.window");
export const WORKSPACE = service<WorkspaceService>("flame.shell.workspace");
export const COMMANDS = service<CommandsService>("flame.shell.commands");

/** Bound per plugin by `definePlugin`; all three carry the plugin's identity. */
export interface AmbientShell {
  notify(message: string, level?: NotificationLevel): void;
  readonly storage: KeyValueStore;
  /** Settled tasks linger so the outcome is seen. */
  startTask(opts: TaskStartOptions): TaskHandle;
}
