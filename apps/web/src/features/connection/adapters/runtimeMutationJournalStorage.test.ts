import { IMAGE_FILENAME_STEM } from "@/app/product";
import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBrowserHost } from "@/platform/browserHost";
import { type KeyValueStore } from "@/extensions/storage";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";
import { createStorage } from "@/extensions/storage";

let connection: ReturnType<typeof createRuntimeConnection>;

beforeEach(async () => {
  connection = createRuntimeConnection(
    createBrowserHost(IMAGE_FILENAME_STEM),
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage(createStorage("test.runtime")),
  );
  await connection.initialize();
});

afterEach(async () => {
  await connection.dispose();
});

describe("Runtime mutation journal storage adapter", () => {
  it("owns verified opaque records in the supplied Runtime storage", () => {
    const stored = new Map<string, unknown>();
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    };
    const storage = createRuntimeMutationJournalStorage(ctx.storage);
    expect(storage).not.toBeNull();
    stored.set("mutation-journal.probe:stale", "stale-probe");
    storage?.set("entry:key-1", { version: 1 });
    expect(storage?.get("entry:key-1")).toEqual({ version: 1 });
    expect(storage?.keys()).toEqual(["entry:key-1"]);
    expect(stored.has("mutation-journal.entry:key-1")).toBe(true);
    storage?.remove("entry:key-1");
    expect(storage?.get("entry:key-1")).toBeUndefined();

    expect(createRuntimeMutationJournalStorage(ctx.storage).keys()).toEqual([]);
  });

  it("ignores records outside the only current storage shape", () => {
    const stored = new Map<string, unknown>([
      ["mutation-journal-v1", { version: 1 }],
      ["mutation-journal-v3.entry:old", { version: 2 }],
    ]);
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => stored.set(key, value),
        remove: (key) => stored.delete(key),
        keys: () => [...stored.keys()],
        clear: () => stored.clear(),
      },
    };
    const storage = createRuntimeMutationJournalStorage(ctx.storage);

    expect(storage?.keys()).toEqual([]);
    expect(storage?.get("entry:old")).toBeUndefined();
  });

  it("surfaces Host storage writes and removals that were swallowed", () => {
    const stored = new Map<string, unknown>();
    let discardWrites = true;
    let discardRemovals = false;
    let hideKeys = false;
    const ctx: { storage: KeyValueStore } = {
      storage: {
        get: (key) => stored.get(key),
        set: (key, value) => {
          if (!discardWrites) stored.set(key, value);
        },
        remove: (key) => {
          if (!discardRemovals) stored.delete(key);
        },
        keys: () => (hideKeys ? [] : [...stored.keys()]),
        clear: () => stored.clear(),
      },
    };
    const storage = createRuntimeMutationJournalStorage(ctx.storage);

    expect(() => storage.set("entry:key-1", { version: 1 })).toThrow("not persisted");
    discardWrites = false;
    storage.set("entry:key-1", { version: 1 });
    hideKeys = true;
    expect(() => storage.keys()).toThrow("enumeration is incomplete");
    hideKeys = false;
    discardRemovals = true;
    expect(() => storage.remove("entry:key-1")).toThrow("not persisted");
  });

  it("captures journal storage before constructing its first client", async () => {
    await connection.dispose();
    const storage = createRuntimeMutationJournalStorage(createStorage("test.runtime"));
    connection = createRuntimeConnection(
      createBrowserHost(IMAGE_FILENAME_STEM),
      runtimeRequestMeta,
      storage,
    );
    await connection.initialize();
    const first = connection.client();
    const close = vi.spyOn(first, "close");
    expect(connection.client()).toBe(first);
    expect(close).not.toHaveBeenCalled();
    await connection.dispose();
    expect(close).toHaveBeenCalledOnce();
  });
});
