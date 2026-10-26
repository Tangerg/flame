import { stringifyJSON } from "@flame/runtime-client";
import { type KeyValueStore } from "@/extensions/storage";
export interface RuntimeMutationJournalStorage {
  get(key: string): unknown;
  set(key: string, value: unknown): void;
  remove(key: string): void;
  keys(): string[];
}
const STORAGE_PREFIX = "mutation-journal.";

const PROBE_STORAGE_PREFIX = `${STORAGE_PREFIX}probe:`;

function storageKey(key: string): string {
  return `${STORAGE_PREFIX}${key}`;
}

function storedValuesEqual(left: unknown, right: unknown): boolean {
  if (left === undefined || right === undefined) return left === right;
  return stringifyJSON(left) === stringifyJSON(right);
}

export function createRuntimeMutationJournalStorage(
  storage: KeyValueStore,
): RuntimeMutationJournalStorage {
  return {
    get: (key) => storage.get(storageKey(key)),
    set: (key, value) => {
      if (value === undefined) {
        throw new Error(`Runtime mutation journal cannot persist undefined: ${key}`);
      }
      const target = storageKey(key);
      storage.set(target, value);
      if (!storedValuesEqual(storage.get(target), value)) {
        throw new Error(`Runtime mutation journal write was not persisted: ${key}`);
      }
    },
    remove: (key) => {
      const target = storageKey(key);
      storage.remove(target);
      if (storage.get(target) !== undefined) {
        throw new Error(`Runtime mutation journal removal was not persisted: ${key}`);
      }
    },
    keys: () => {
      const probeStorageKey = `${PROBE_STORAGE_PREFIX}${crypto.randomUUID()}`;
      const probe = crypto.randomUUID();
      storage.set(probeStorageKey, probe);
      if (storage.get(probeStorageKey) !== probe) {
        storage.remove(probeStorageKey);
        if (storage.get(probeStorageKey) !== undefined) {
          throw new Error("Runtime mutation journal failed probe was not removed");
        }
        throw new Error("Runtime mutation journal storage is not writable");
      }
      let keys: string[] = [];
      let enumerationError: unknown;
      try {
        keys = storage.keys();
        if (!keys.includes(probeStorageKey)) {
          throw new Error("Runtime mutation journal storage enumeration is incomplete");
        }
      } catch (error) {
        enumerationError = error;
      }
      storage.remove(probeStorageKey);
      if (storage.get(probeStorageKey) !== undefined) {
        throw new Error("Runtime mutation journal storage probe was not removed");
      }
      if (enumerationError !== undefined) throw enumerationError;
      return keys
        .filter((key) => key.startsWith(STORAGE_PREFIX) && !key.startsWith(PROBE_STORAGE_PREFIX))
        .map((key) => key.slice(STORAGE_PREFIX.length));
    },
  };
}
