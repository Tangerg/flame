import { createStorage } from "@/extensions/storage";
import { runtimeRequestMeta } from "@/app/runtimeProtocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createBrowserHost } from "@/platform/browserHost";
import { createRuntimeMutationJournalStorage } from "@/features/connection/adapters/runtimeMutationJournalStorage";

let connection: ReturnType<typeof createRuntimeConnection>;

beforeEach(async () => {
  connection = createRuntimeConnection(
    createBrowserHost(),
    runtimeRequestMeta,
    createRuntimeMutationJournalStorage("test.runtime"),
  );
  await connection.initialize();
});

afterEach(async () => {
  await connection.dispose();
});

describe("Runtime mutation journal storage adapter", () => {
  it("can inspect and retire existing commands when new writes exceed quota", () => {
    const backing = window.localStorage;
    const write = vi.fn((key: string, value: string) => backing.setItem(key, value));
    const port: Storage = {
      get length() {
        return backing.length;
      },
      clear: () => backing.clear(),
      key: (index) => backing.key(index),
      getItem: (key) => backing.getItem(key),
      removeItem: (key) => backing.removeItem(key),
      setItem: write,
    };
    const storage = createRuntimeMutationJournalStorage("quota-test", () => port);
    storage.set("entry:existing", { command: "original" });
    const quota = new DOMException("Quota exceeded", "QuotaExceededError");
    write.mockClear().mockImplementation(() => {
      throw quota;
    });
    expect(storage.keys()).toEqual(["entry:existing"]);
    expect(storage.get("entry:existing")).toEqual({ command: "original" });
    expect(write).not.toHaveBeenCalled();
    expect(() => storage.set("entry:new", "x")).toThrow("Quota exceeded");
    storage.remove("entry:existing");
    expect(storage.keys()).toEqual([]);
  });

  it("preserves the cause of failed reads, enumeration and removal", () => {
    const cause = new Error("storage unavailable");
    const storage = createRuntimeMutationJournalStorage("test", () => {
      throw cause;
    });
    expect(() => storage.keys()).toThrow(cause);
    expect(() => storage.get("entry:1")).toThrow(cause);
    expect(() => storage.remove("entry:1")).toThrow(cause);
  });

  it("captures journal storage before constructing its first client", async () => {
    await connection.dispose();
    const storage = createRuntimeMutationJournalStorage("test.runtime");
    connection = createRuntimeConnection(createBrowserHost(), runtimeRequestMeta, storage);
    await connection.initialize();
    const first = connection.client();
    const close = vi.spyOn(first, "close");
    expect(connection.client()).toBe(first);
    expect(close).not.toHaveBeenCalled();
    await connection.dispose();
    expect(close).toHaveBeenCalledOnce();
  });
});

it("keeps pending journal evidence from the original namespace readable", () => {
  const prior = createStorage("retained");
  prior.set("mutation-journal.entry:one", { evidence: "original" });
  const current = createRuntimeMutationJournalStorage("retained");
  expect(current.get("entry:one")).toEqual({ evidence: "original" });
  current.remove("entry:one");
});
