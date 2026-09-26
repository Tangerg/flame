import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_SERVER_SCOPE } from "@/plugins/builtin/runtime/public/services";
import * as runtimeEndpoint from "@/plugins/builtin/runtime/public/endpoint";
import { composerBootstrap } from "./bootstrap";

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
          },
        };
      },
    });

    await loadPluginsForTest(composerBootstrap, sessions);

    expect(lifecycleSnapshot).toHaveBeenCalledOnce();
    expect(subscribeLifecycle).toHaveBeenCalledOnce();
  });
});
