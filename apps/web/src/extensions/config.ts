import { type Disposable } from "@/extensions/types/lifetime";
import { safeCall } from "@/extensions/errors";
import { service } from "dougong";

export type ConfigValue =
  string | number | boolean | null | ConfigValue[] | { [key: string]: ConfigValue };

export interface ConfigService {
  get(key: string, defaultValue?: ConfigValue): ConfigValue | undefined;
  set(key: string, value: ConfigValue): void;
  has(key: string): boolean;
  onChange(key: string, fn: (value: ConfigValue | undefined) => void): Disposable;
}

export class Configuration implements ConfigService {
  readonly #values = new Map<string, ConfigValue>();
  readonly #subscribers = new Map<string, Set<(value: ConfigValue | undefined) => void>>();

  get(key: string, defaultValue?: ConfigValue): ConfigValue | undefined {
    const value = this.#values.get(key);
    return structuredClone(value === undefined ? defaultValue : value);
  }

  has(key: string): boolean {
    return this.#values.has(key);
  }

  set(key: string, value: ConfigValue): void {
    if (Object.is(this.#values.get(key), value)) return;
    const retained = structuredClone(value);
    this.#values.set(key, retained);
    for (const notify of [...(this.#subscribers.get(key) ?? [])]) {
      safeCall(
        () => notify(structuredClone(retained)),
        `[plugin] config subscriber for "${key}" threw:`,
      );
    }
  }

  onChange(key: string, notify: (value: ConfigValue | undefined) => void): Disposable {
    let subscribers = this.#subscribers.get(key);
    if (!subscribers) {
      subscribers = new Set();
      this.#subscribers.set(key, subscribers);
    }
    subscribers.add(notify);
    const owned = subscribers;
    return {
      dispose: () => {
        owned.delete(notify);
        if (owned.size === 0 && this.#subscribers.get(key) === owned) {
          this.#subscribers.delete(key);
        }
      },
    };
  }
}

export const CONFIG = service<ConfigService>("flame.shell.config");
