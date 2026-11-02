import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import { tupleKey } from "@/lib/tupleKey";
const ROOT = "flame.plugin";

export interface KeyValueStore {
  get: (key: string) => unknown;
  set: (key: string, value: unknown) => void;
  remove: (key: string) => void;
  clear: () => void;
  keys: () => string[];
}

export class PluginStorageError extends Error {}

function accessStorage<T>(operation: (storage: Storage) => T): T {
  try {
    return operation(window.localStorage);
  } catch (cause) {
    throw new PluginStorageError("Plugin preferences are unavailable", { cause });
  }
}

export function pluginStoragePrefix(pluginName: string): string {
  return `${tupleKey(ROOT, pluginName)}:`;
}

export function createStorage(pluginName: string): KeyValueStore {
  const prefix = pluginStoragePrefix(pluginName);

  return {
    get(key: string): unknown {
      return accessStorage<unknown>((storage) => {
        const raw = storage.getItem(prefix + key);
        if (raw == null) return undefined;
        return parseJSON(raw);
      });
    },

    set(key: string, value: unknown): void {
      accessStorage((storage) => {
        storage.setItem(prefix + key, stringifyJSON(value));
      });
    },

    remove(key: string): void {
      accessStorage((storage) => {
        storage.removeItem(prefix + key);
      });
    },

    clear(): void {
      accessStorage((storage) => {
        const doomed: string[] = [];
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i);
          if (key?.startsWith(prefix)) doomed.push(key);
        }
        for (const key of doomed) storage.removeItem(key);
      });
    },

    keys(): string[] {
      return accessStorage<string[]>((storage) => {
        const keys: string[] = [];
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i);
          if (key?.startsWith(prefix)) keys.push(key.slice(prefix.length));
        }
        return keys;
      });
    },
  };
}
