import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { RUNTIME_SERVER_SCOPE } from "@/features/connection/public/services";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { createComposerBootstrap } from "@/features/composer/bootstrap";
afterEach(async () => {
  await resetKernelForTest();
  vi.restoreAllMocks();
});

describe("Composer bootstrap", () => {
  it("reads setup-time session state from its injected session instance", async () => {
    const lifecycleSnapshot = vi.fn(() => ({
      activeSessionId: "session-from-service",
      openSessionIds: ["session-from-service"],
    }));
    const subscribeLifecycle = vi.fn(() => () => undefined);
    const sessionAccess = {
      getActiveSessionId: () => "session-from-service",
      getLifecycleSnapshot: lifecycleSnapshot,
      subscribeActiveSessionId: () => () => undefined,
      subscribeLifecycle,
      subscribeDeleted: () => () => undefined,
    };
    const scope = { subscribeReplacement: () => () => undefined };
    const composer = createComposer(sessionAccess, () => "https://bootstrap-composer.test", scope);
    const sessions = definePlugin({
      name: "test.composer-session-ports",
      provides: { sessions: AGENT_SESSIONS, scope: RUNTIME_SERVER_SCOPE },
      setup: () => ({ scope, sessions: sessionAccess }),
    });
    await loadPluginsForTest(createComposerBootstrap(composer), sessions);

    expect(lifecycleSnapshot).toHaveBeenCalledOnce();
    expect(subscribeLifecycle).toHaveBeenCalledOnce();
  });
});
