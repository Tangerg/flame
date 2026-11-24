import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import { pluginStoragePrefix } from "@/extensions/storage";
import { RUNTIME_PLUGIN } from "@/features/connection/public/services";
import type { MutationJournalStorage } from "@flame/runtime-client";

// Unsettled commands must remain readable when storage is full. The durable
// boundary propagates browser errors and never writes to probe a read.
export function createRuntimeMutationJournalStorage(
  open: () => Storage = () => window.localStorage,
): MutationJournalStorage {
  const prefix = `${pluginStoragePrefix(RUNTIME_PLUGIN)}mutation-journal.`;
  return {
    get(key) {
      const raw = open().getItem(prefix + key);
      return raw === null ? undefined : parseJSON(raw);
    },
    set(key, value) {
      open().setItem(prefix + key, stringifyJSON(value));
    },
    remove(key) {
      open().removeItem(prefix + key);
    },
    keys() {
      const storage = open();
      const keys: string[] = [];
      for (let i = 0; i < storage.length; i++) {
        const key = storage.key(i);
        if (key?.startsWith(prefix)) keys.push(key.slice(prefix.length));
      }
      return keys;
    },
  };
}
