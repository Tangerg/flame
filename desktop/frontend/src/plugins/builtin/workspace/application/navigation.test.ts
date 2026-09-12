import { beforeEach, describe, expect, it } from "vitest";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { navigator } from "@/lib/navigation";
import {
  WORKSPACE_DOCK_CATALOG,
  activateWorkspaceSessionScope,
  closeActiveWorkspaceDockView,
  closeAllWorkspaceDockViews,
  closeWorkspaceDockView,
  closeActiveWorkspaceView,
  collapseWorkspaceDock,
  locateWorkspaceTool,
  openWorkspaceView,
  openWorkspaceViewInDock,
  selectWorkspaceDockView,
  showWorkspaceDock,
  toggleWorkspaceDock,
} from "./navigation";

function reset() {
  document.body.replaceChildren();
  navigator().go({ view: "v2", settings: null });
  useContextDockStore.setState({
    activeSessionScopeId: null,
    sessionScopes: new Map(),
    dockViewIds: [],
    lastViewId: null,
    fileFocus: WorkspaceFileFocus.empty(),
    fileViewer: null,
    expandedToolIds: new Set<string>(),
  });
}

describe("workspace navigation port", () => {
  beforeEach(reset);

  it("opens dock views as stable singleton tabs beside chat", () => {
    openWorkspaceViewInDock("skills");
    openWorkspaceViewInDock("diff");
    openWorkspaceViewInDock("skills");

    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["skills", "diff"],
      lastViewId: "skills",
    });
  });

  it("only selects views that already belong to the dock", () => {
    openWorkspaceViewInDock("skills");
    selectWorkspaceDockView("diff");
    expect(navigator().get().dock).toBe("skills");
  });

  it("a full view leaves the dock workspace alone", () => {
    openWorkspaceViewInDock("skills");
    collapseWorkspaceDock();
    openWorkspaceView("v3");

    expect(navigator().get().view).toBe("v3");
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["skills"],
      lastViewId: "skills",
    });
  });

  it("closing a full view returns to chat without changing dock tabs", () => {
    openWorkspaceViewInDock("skills");
    openWorkspaceView("v3");

    expect(closeActiveWorkspaceView()).toBe(true);
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["skills"]);
  });

  it("collapse and show are a lossless round trip", () => {
    openWorkspaceViewInDock("diff");
    collapseWorkspaceDock();
    showWorkspaceDock();

    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("showing an empty dock offers the catalogue rather than choosing a panel", () => {
    showWorkspaceDock();

    expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: [],
      lastViewId: WORKSPACE_DOCK_CATALOG,
    });
  });

  it("returns an emptied dock to its catalogue rather than closing it", () => {
    openWorkspaceViewInDock("diff");

    closeWorkspaceDockView("diff");

    expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  });

  it("returns to the catalogue when every tab is closed at once", () => {
    openWorkspaceViewInDock("diff");
    openWorkspaceViewInDock("plan");

    closeAllWorkspaceDockViews();

    expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  });

  it("restores the catalogue after renderer replacement without opening a phantom tab", () => {
    navigator().go({ session: "s1", dock: WORKSPACE_DOCK_CATALOG });

    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);
  });

  it("does not resurrect the last closed tab when returning to a session", () => {
    activateWorkspaceSessionScope("s1");
    openWorkspaceViewInDock("diff");
    closeWorkspaceDockView("diff");

    navigator().go({ session: "s2" });
    activateWorkspaceSessionScope("s2");
    navigator().go({ session: "s1" });
    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);
  });

  it("closes an empty dock through the active-view close command", () => {
    showWorkspaceDock();

    expect(closeActiveWorkspaceDockView()).toBe(true);

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual([]);
  });

  it("shows the remembered panel instead of the catalogue when there is one", () => {
    openWorkspaceViewInDock("diff");
    collapseWorkspaceDock();

    showWorkspaceDock();

    expect(navigator().get().dock).toBe("diff");
  });

  it("toggles the visible dock without discarding its session tabs", () => {
    openWorkspaceViewInDock("diff");

    toggleWorkspaceDock();
    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);

    toggleWorkspaceDock();
    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("does not reopen a collapsed dock when the same session scope is rebound", () => {
    navigator().go({ session: "s1", dock: null });
    useContextDockStore.setState({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("distinguishes a real move from the initialized sessionless scope", () => {
    navigator().go({ session: "s1", dock: "diff" });
    useContextDockStore.setState({
      activeSessionScopeId: "",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: [],
      lastViewId: null,
    });
  });

  it("closes the active dock tab before the session-level command can run", () => {
    openWorkspaceViewInDock("skills");
    openWorkspaceViewInDock("diff");

    expect(closeActiveWorkspaceDockView()).toBe(true);
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["skills"],
      lastViewId: "skills",
    });
  });

  it("locates a parent task by selecting chat and atomically revealing its tool", () => {
    const anchor = document.createElement("div");
    anchor.id = "task-item";
    anchor.scrollIntoView = () => {};
    const button = document.createElement("button");
    anchor.append(button);
    document.body.append(anchor);

    locateWorkspaceTool("task-item");

    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().expandedToolIds).toEqual(new Set(["task-item"]));
    expect(document.activeElement).toBe(button);
  });

  it("keeps looking when the anchor is mounted before its control", async () => {
    const anchor = document.createElement("div");
    anchor.id = "late-item";
    anchor.scrollIntoView = () => {};
    document.body.append(anchor);

    locateWorkspaceTool("late-item");
    expect(document.activeElement).toBe(document.body);

    const button = document.createElement("button");
    anchor.append(button);
    await new Promise((resolve) => requestAnimationFrame(resolve));

    expect(document.activeElement).toBe(button);
  });
});
