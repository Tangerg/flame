import { beforeEach, describe, expect, it, vi } from "vitest";
import { useContextDockStore } from "./adapters/contextDockStore";
import { navigator } from "@/lib/navigation";
import { workspaceSessionNavigation as sessionNavigation } from "./sessionNavigation";
import { definePlugin } from "@/plugins/sdk";
import { AGENT_SESSIONS } from "@/plugins/builtin/agent/public/services";
import { WORKSPACE_SCOPE } from "@/plugins/builtin/workspace/public/services";
import {
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  forgetWorkspaceSessionScopes,
} from "@/plugins/builtin/workspace/public/navigation";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";

const agentSession = vi.hoisted(() => {
  let listener: ((sessionId: string) => void) | undefined;
  let openSessionIds = ["s1", "s2"];
  return {
    goTo(sessionId: string) {
      navigator().go({ session: sessionId });
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
      getActiveSessionId: () => navigator().get().session,
      getLifecycleSnapshot: () => ({
        activeSessionId: navigator().get().session,
        openSessionIds: agentSession.getOpen(),
      }),
      subscribeActiveSessionId: (fn: (sessionId: string) => void) => agentSession.subscribe(fn),
      subscribeLifecycle: () => () => {},
    },
    scopes: {
      adoptSessionScope: adoptWorkspaceSessionScope,
      activateSessionScope: activateWorkspaceSessionScope,
      forgetSessionScopes: forgetWorkspaceSessionScopes,
    },
  }),
});

beforeEach(() => {
  agentSession.reset();
  navigator().go({ session: "s1" });
});

describe("workspace session navigation", () => {
  it("adopts the session the app is already in when it loads", async () => {
    useContextDockStore.setState({ activeSessionScopeId: null, sessionScopes: new Map() });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });

  it("adopts the dock location that survived a renderer replacement", async () => {
    navigator().go({ session: "s1", dock: "diff" });
    useContextDockStore.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
      dockViewIds: [],
      lastViewId: null,
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("opens a subagent deep link before the open-session list has hydrated", async () => {
    agentSession.setOpen([]);
    navigator().go({ session: "s1", dock: "subagents", subagent: "child" });
    useContextDockStore.setState({
      activeSessionScopeId: null,
      sessionScopes: new Map(),
      dockViewIds: [],
      lastViewId: null,
    });

    await loadPluginsForTest(ports, sessionNavigation);

    expect(navigator().get()).toMatchObject({
      session: "s1",
      dock: "subagents",
      subagent: "child",
    });
    expect(useContextDockStore.getState().dockViewIds).toEqual(["subagents"]);
  });

  it("restores the dock destination the session it moves to remembers", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    useContextDockStore.getState().adoptDockLocation("diff");

    agentSession.goTo("s2");
    expect(navigator().get().dock).toBeNull();

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
    expect(navigator().get().dock).toBe("diff");
  });

  // A session-list reconciliation drops the active scope pointer while the app
  // keeps running. That used to read the same as a cold start, so the next move
  // adopted the dock of the session it was leaving instead of restoring the one
  // it was entering — the right pane opening on its own, on the sessions that
  // happened to be reconciled first.
  it("does not carry the dock into the session it moves to after a reconciliation", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    useContextDockStore.getState().adoptDockLocation("diff");
    navigator().go({ dock: "diff" });

    forgetWorkspaceSessionScopes([]);
    expect(useContextDockStore.getState().activeSessionScopeId).toBeNull();

    agentSession.goTo("s2");

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);
  });

  it("keeps each session's own tabs", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    useContextDockStore.getState().adoptDockLocation("diff");

    agentSession.goTo("s2");
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);

    agentSession.goTo("s1");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("stops following once unloaded", async () => {
    await loadPluginsForTest(ports, sessionNavigation);
    await resetKernelForTest();

    agentSession.goTo("s2");

    expect(useContextDockStore.getState().activeSessionScopeId).toBe("s1");
  });
});
