import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { beforeEach, describe, expect, it, vi } from "vitest";

import { workspaceSessionNavigation as sessionNavigation } from "@/features/workspace/sessionNavigation";
import { definePlugin } from "@/extensions/definePlugin";
import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { WORKSPACE_SCOPE } from "@/features/workspace/public/services";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/features/workspace/public/navigation";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
const agentSession = vi.hoisted(() => {
  let listener: ((sessionId: string) => void) | undefined;
  let openSessionIds = ["s1", "s2"];
  return {
    goTo(sessionId: string) {
      testClient.navigation.go({ session: sessionId });
      listener?.(sessionId);
    },
    setOpen(ids: string[]) {
      openSessionIds = ids;
    },
    getOpen: () => openSessionIds,
    subscribe(fn: (sessionId: string) => void) {
      listener = fn;
      return () => {
        listener = undefined;
      };
    },
    reset() {
      listener = undefined;
      openSessionIds = ["s1", "s2"];
    },
  };
});

const ports = definePlugin({
  name: "test.session-ports",
  provides: { sessions: AGENT_SESSIONS, scopes: WORKSPACE_SCOPE },
  setup: () => ({
    sessions: {
      getActiveSessionId: () => testClient.navigation.get().session,
      getLifecycleSnapshot: () => ({
        activeSessionId: testClient.navigation.get().session,
        openSessionIds: agentSession.getOpen(),
      }),
      subscribeActiveSessionId: (fn: (sessionId: string) => void) => agentSession.subscribe(fn),
      subscribeLifecycle: () => () => {},
      subscribeDeleted: () => () => {},
    },
    scopes: {
      adoptSessionScope: adoptWorkspaceSessionScope.bind(null, testClient.workspace.access),
      activateSessionScope: activateWorkspaceSessionScope.bind(null, testClient.workspace.access),
      forgetSessionScopes: forgetWorkspaceSessionScopes.bind(null, testClient.workspace.access),
    },
  }),
});

beforeEach(() => {
  agentSession.reset();
  testClient.navigation.go({ session: "s1" });
});

describe("workspace session navigation", () => {
  it("adopts the session the app is already in when it loads", async () => {
    testClient.workspace.dock.store.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(testClient.workspace.dock.store.getState().activeSessionScopeId).toBe("s1");
  });

  it("adopts the dock location that survived a renderer replacement", async () => {
    testClient.navigation.go({ session: "s1", dock: "diff" });
    testClient.workspace.dock.store.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
      dockViewIds: [],
      lastViewId: null,
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(testClient.navigation.get().dock).toBe("diff");
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("opens a subagent deep link before the open-session list has hydrated", async () => {
    agentSession.setOpen([]);
    testClient.navigation.go({ session: "s1", dock: "subagents", subagent: "child" });
    testClient.workspace.dock.store.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
      dockViewIds: [],
      lastViewId: null,
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(testClient.navigation.get()).toMatchObject({
      session: "s1",
      dock: "subagents",
      subagent: "child",
    });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["subagents"]);
  });

  it("restores the dock destination the session it moves to remembers", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    testClient.workspace.dock.store.getState().adoptDockLocation("diff");

    agentSession.goTo("s2");
    expect(testClient.navigation.get().dock).toBeNull();

    agentSession.goTo("s1");
    expect(testClient.workspace.dock.store.getState().activeSessionScopeId).toBe("s1");
    expect(testClient.navigation.get().dock).toBe("diff");
  });

  // A session-list reconciliation drops the active scope pointer while the app
  // keeps running. That used to read the same as a cold start, so the next move
  // adopted the dock of the session it was leaving instead of restoring the one
  // it was entering — the right pane opening on its own, on the sessions that
  // happened to be reconciled first.
  it("does not carry the dock into the session it moves to after a reconciliation", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    testClient.workspace.dock.store.getState().adoptDockLocation("diff");
    testClient.navigation.go({ dock: "diff" });

    forgetWorkspaceSessionScopes(testClient.workspace.access, []);
    expect(testClient.workspace.dock.store.getState().activeSessionScopeId).toBeNull();

    agentSession.goTo("s2");

    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);
  });

  it("keeps each session's own tabs", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    testClient.workspace.dock.store.getState().adoptDockLocation("diff");

    agentSession.goTo("s2");
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);

    agentSession.goTo("s1");
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["diff"]);
  });

  it("stops following once unloaded", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    await resetKernelForTest();

    agentSession.goTo("s2");

    expect(testClient.workspace.dock.store.getState().activeSessionScopeId).toBe("s1");
  });
});
