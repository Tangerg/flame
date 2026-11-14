import { afterEach, beforeEach } from "vitest";
import "fake-indexeddb/auto";
import { IDBFactory } from "fake-indexeddb";
import { MotionGlobalConfig } from "motion/react";
import { usePluginErrorStore } from "@/extensions/errors";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { resetKernelForTest } from "@/test/kernel";
MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  globalThis.indexedDB = new IDBFactory();
  localStorage.clear();
  usePluginErrorStore.setState({ log: [] });
  useNotificationStore.setState({ log: [] });
});

afterEach(() => {
  try {
    localStorage.clear();
  } catch {}
});
