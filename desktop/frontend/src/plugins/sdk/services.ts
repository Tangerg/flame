// What a plugin declares in `requires` to reach a capability the shell owns. Anything that
// is really "a point plus a call to `ctx.contribute`" does NOT belong here.
//
// `ctx.notify` and `ctx.storage` live on the plugin context instead: they are ambient and
// identity-scoped, so there is no provider to declare.

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
  /**
   * Plugin keys live ALONGSIDE the kernel's and resolve through `t()` normally. Last
   * writer wins on collision.
   */
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
  /**
   * Run a command by id — the lightweight cross-plugin call. Warns and no-ops
   * on an unknown id.
   */
  execute(id: string, ...args: unknown[]): Promise<void>;
}

export const CONFIG = service<ConfigService>("flame.shell.config");
export const I18N = service<I18nService>("flame.shell.i18n");
export const WINDOW = service<WindowService>("flame.shell.window");
export const WORKSPACE = service<WorkspaceService>("flame.shell.workspace");
export const COMMANDS = service<CommandsService>("flame.shell.commands");

/** The ambient half, bound per plugin by `definePlugin` — all three carry the
 *  plugin's identity, so there is no provider to declare. */
export interface AmbientShell {
  notify(message: string, level?: NotificationLevel): void;
  readonly storage: KeyValueStore;
  /** Register a long-running task; settled ones linger so the outcome is seen. */
  startTask(opts: TaskStartOptions): TaskHandle;
}
