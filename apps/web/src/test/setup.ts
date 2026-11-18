import { afterEach, beforeEach } from "vitest";
import "fake-indexeddb/auto";
import { IDBFactory } from "fake-indexeddb";
import { usePluginErrorStore } from "@/extensions/errors";
import { resetKernelForTest } from "@/test/kernel";

beforeEach(async () => {
  await resetKernelForTest();
  globalThis.indexedDB = new IDBFactory();
  localStorage.clear();
  usePluginErrorStore.setState({ log: [] });
});

afterEach(() => {
  try {
    localStorage.clear();
  } catch {}
});
