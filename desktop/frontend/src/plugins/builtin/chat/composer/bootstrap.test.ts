import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { composerBootstrap } from "./bootstrap";

afterEach(async () => {
  await resetKernelForTest();
});

describe("Composer bootstrap", () => {
  it("reads setup-time session state from its declared service", async () => {
    const lifecycleSnapshot = vi.fn(() => ({
      activeSessionId: "session-from-service",
      openSessionIds: ["session-from-service"],
    }));
    const subscribeLifecycle = vi.fn(() => () => undefined);
    const sessions = definePlugin({
      name: "test.composer-session-ports",
      provides: { sessions: AGENT_SESSION_PORTS },
      setup() {
        return {
          sessions: {
            getActiveSessionId: () => "session-from-service",
            getLifecycleSnapshot: lifecycleSnapshot,
            subscribeActiveSessionId: () => () => undefined,
            subscribeLifecycle,
          },
        };
      },
    });

    await loadPluginsForTest(composerBootstrap, sessions);

    expect(lifecycleSnapshot).toHaveBeenCalledOnce();
    expect(subscribeLifecycle).toHaveBeenCalledOnce();
  });
});
