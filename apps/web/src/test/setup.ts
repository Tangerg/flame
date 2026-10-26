import { afterEach, beforeEach } from "vitest";
import { MotionGlobalConfig } from "motion/react";
import { useConfigStore } from "@/extensions/config";
import { usePluginErrorStore } from "@/extensions/errors";
import { useNotificationStore } from "@/features/notifications/public/notifications";
import { resetKernelForTest } from "@/test/kernel";
MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  localStorage.clear();
  usePluginErrorStore.setState({ log: [] });
  useNotificationStore.setState({ log: [] });
  useConfigStore.setState({ values: new Map(), subscribers: new Map() });
});

afterEach(() => {
  try {
    localStorage.clear();
  } catch {}
});
