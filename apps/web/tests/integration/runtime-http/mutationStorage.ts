import type { MutationJournalStorage } from "@flame/runtime-client";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";

export class MemoryMutationStorage implements MutationJournalStorage {
  readonly #entries = new Map<string, string>();

  get size(): number {
    return this.#entries.size;
  }

  get(key: string): unknown {
    const value = this.#entries.get(key);
    return value === undefined ? undefined : parseJSON(value);
  }

  set(key: string, value: unknown): void {
    this.#entries.set(key, stringifyJSON(value));
  }

  remove(key: string): void {
    this.#entries.delete(key);
  }

  keys(): string[] {
    return [...this.#entries.keys()];
  }

  snapshot(): [string, string][] {
    return [...this.#entries];
  }
}
