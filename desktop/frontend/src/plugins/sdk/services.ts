import { service } from "dougong";
import type { ConfigValue } from "./config";
import type { KeyValueStore } from "./storage";
import type { Disposable } from "./types/common";
import type { NotificationLevel, TaskHandle, TaskStartOptions } from "./types/infra";

export interface ConfigService {
  get(key: string, defaultValue?: ConfigValue): ConfigValue | undefined;
  set(key: string, value: ConfigValue): void;
  has(key: string): boolean;
  onChange(key: string, fn: (value: ConfigValue | undefined) => void): Disposable;
}

export interface I18nService {
  addBundle(locale: string, dict: Record<string, string>): void;
}

export interface WindowService {
  setTitle(text: string): void;
  setBadge(n?: number): void;
  setWorking(on: boolean): void;
}

export interface WorkspaceService {
  openView(id: string): void;
  closeView(id: string): void;
}

export interface CommandsService {
  execute(id: string, ...args: unknown[]): Promise<void>;
}

export const CONFIG = service<ConfigService>("flame.shell.config");
export const I18N = service<I18nService>("flame.shell.i18n");
export const WINDOW = service<WindowService>("flame.shell.window");
export const WORKSPACE = service<WorkspaceService>("flame.shell.workspace");
export const COMMANDS = service<CommandsService>("flame.shell.commands");

export interface AmbientShell {
  notify(message: string, level?: NotificationLevel): void;
  readonly storage: KeyValueStore;
  startTask(opts: TaskStartOptions): TaskHandle;
}
