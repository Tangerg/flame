import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
import * as runtimeEndpoint from "@/features/connection/public/endpoint";
import { composerBootstrap } from "@/features/composer/bootstrap";
afterEach(async () => {
  await resetKernelForTest();
  vi.restoreAllMocks();
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
      provides: { sessions: AGENT_SESSIONS, scope: RUNTIME_SERVER_SCOPE },
      setup() {
        vi.spyOn(runtimeEndpoint, "currentRuntimeEndpoint").mockReturnValue(
          "https://bootstrap-composer.test",
        );
        return {
          scope: { subscribeReplacement: () => () => undefined },
          sessions: {
            getActiveSessionId: () => "session-from-service",
            getLifecycleSnapshot: lifecycleSnapshot,
            subscribeActiveSessionId: () => () => undefined,
            subscribeLifecycle,
            subscribeDeleted: () => () => undefined,
          },
        };
      },
    });

    await loadPluginsForTest(composerBootstrap, sessions);

    expect(lifecycleSnapshot).toHaveBeenCalledOnce();
    expect(subscribeLifecycle).toHaveBeenCalledOnce();
  });
});
