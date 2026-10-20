import { contributeForTest } from "@/plugins/sdk/testKernel";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { beforeEach, describe, expect, it } from "vitest";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { navigator } from "@/lib/navigation";
import {
  WORKSPACE_DOCK_CATALOG,
  openWorkspaceSubagentRun,
  openWorkspaceFile,
  returnFromWorkspaceInspection,
  activateWorkspaceSessionScope,
  adoptWorkspaceSessionScope,
  closeActiveWorkspaceDockView,
  closeAllWorkspaceDockViews,
  closeWorkspaceDockView,
  closeActiveWorkspaceView,
  collapseWorkspaceDock,
  openWorkspaceView,
  openWorkspaceViewInDock,
  selectWorkspaceDockView,
  showWorkspaceDock,
  toggleWorkspaceDock,
  openWorkspaceDiffForFile,
  openWorkspaceClientDiagnostics,
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

    adoptWorkspaceSessionScope("s1");

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
});

it("keeps subagent selection in session-scoped history without replacing the main conversation", () => {
  navigator().go({ session: "parent-session" });
  openWorkspaceSubagentRun("child-run");
  expect(navigator().get()).toMatchObject({
    session: "parent-session",
    view: null,
    dock: "subagents",
    subagent: "child-run",
  });
  openWorkspaceSubagentRun("nested-run");
  navigator().back();
  expect(navigator().get().subagent).toBe("child-run");
  navigator().go({ session: "other-session" });
  expect(navigator().get().subagent).toBeNull();
  navigator().back();
  expect(navigator().get()).toMatchObject({ session: "parent-session", subagent: "child-run" });
});

it("keeps subagent selection and the way back in the active full view", () => {
  navigator().go({
    session: "parent-session",
    view: "subagents",
    dock: "diff",
    subagent: null,
  });

  openWorkspaceSubagentRun("child-run");
  expect(navigator().get()).toMatchObject({
    session: "parent-session",
    view: "subagents",
    dock: "diff",
    subagent: "child-run",
  });

  openWorkspaceSubagentRun("nested-run");
  navigator().back();
  expect(navigator().get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: "child-run",
  });

  openWorkspaceSubagentRun(null);
  expect(navigator().get()).toMatchObject({ view: "subagents", dock: "diff", subagent: null });
});

it("restores the file inspected before a task opened a different file", () => {
  navigator().go({ session: "session", view: null });
  openWorkspaceFile("src/original.ts", 12);
  const file = document.createElement("div");
  file.dataset.dockViewId = "file";
  const source = document.createElement("button");
  file.append(source);
  document.body.append(file);
  openWorkspaceSubagentRun("child", source);
  const fromFile = useContextDockStore.getState().inspection!;
  const task = document.createElement("div");
  task.dataset.dockViewId = "subagents";
  const nested = document.createElement("button");
  task.append(nested);
  document.body.append(task);
  openWorkspaceFile("src/evidence.ts", 40, nested);
  const fromTask = useContextDockStore.getState().inspection!;
  expect(returnFromWorkspaceInspection(fromTask)).toBe(true);
  expect(navigator().get()).toMatchObject({ dock: "subagents", subagent: "child" });
  expect(returnFromWorkspaceInspection(fromFile)).toBe(true);
  expect(navigator().get()).toMatchObject({ dock: "file", subagent: "child" });
  expect(useContextDockStore.getState().fileViewer).toEqual({ path: "src/original.ts", line: 12 });
  expect(useContextDockStore.getState().inspection).toBeNull();
  expect(returnFromWorkspaceInspection(fromTask)).toBe(false);
  file.remove();
  task.remove();
});

it("opens the contribution catalogue when Diff has no admitted renderer or several choices", async () => {
  openWorkspaceDiffForFile("a.ts");
  expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  await contributeForTest((ctx) => {
    for (const id of ["package:first:diff", "package:second:diff"])
      ctx.contribute(WORKSPACE_VIEW, {
        id,
        title: id,
        material: "diff",
        dock: "workspace",
        component: () => null,
      });
  });
  openWorkspaceDiffForFile("b.ts");
  expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
});

it("opens only a unique admitted client diagnostics contribution", async () => {
  openWorkspaceClientDiagnostics();
  expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  await contributeForTest((ctx) =>
    ctx.contribute(WORKSPACE_VIEW, {
      id: "package:first:diagnostics",
      title: "Diagnostics",
      material: "diagnostics",
      dock: "workspace",
      component: () => null,
    }),
  );
  openWorkspaceClientDiagnostics();
  expect(navigator().get().dock).toBe("package:first:diagnostics");
  await contributeForTest((ctx) =>
    ctx.contribute(WORKSPACE_VIEW, {
      id: "package:second:diagnostics",
      title: "Other diagnostics",
      material: "diagnostics",
      dock: "workspace",
      component: () => null,
    }),
  );
  openWorkspaceClientDiagnostics();
  expect(navigator().get().dock).toBe(WORKSPACE_DOCK_CATALOG);
});

it("keeps a presented Files page while its tree and line navigation change the file", () => {
  navigator().go({ session: "session", view: "file", dock: "diff" });
  openWorkspaceFile("src/tree.ts");
  openWorkspaceFile("src/tree.ts", 24);
  expect(navigator().get()).toMatchObject({ view: "file", dock: "diff" });
  expect(useContextDockStore.getState()).toMatchObject({
    fileViewer: { path: "src/tree.ts", line: 24 },
    inspection: null,
  });

  navigator().go({ view: "v3" });
  const source = document.createElement("button");
  document.body.append(source);
  openWorkspaceFile("src/inspected.ts", 3, source);
  expect(navigator().get()).toMatchObject({ view: null, dock: "file" });
  expect(useContextDockStore.getState().inspection).toMatchObject({ returnViewId: "v3" });
  source.remove();
});

it("restores an ordinary Dock source full width only when the return asks for it", () => {
  navigator().go({ session: "session", view: null });
  openWorkspaceViewInDock("skills");
  const dock = document.createElement("div");
  dock.dataset.dockViewId = "skills";
  const source = document.createElement("button");
  dock.append(source);
  document.body.append(dock);

  openWorkspaceFile("src/a.ts", 1, source);
  expect(returnFromWorkspaceInspection(useContextDockStore.getState().inspection!)).toBe(true);
  expect(navigator().get()).toMatchObject({ view: null, dock: "skills" });

  openWorkspaceFile("src/a.ts", 1, source);
  const inspection = useContextDockStore.getState().inspection!;
  expect(returnFromWorkspaceInspection(inspection, "main")).toBe(true);
  expect(navigator().get()).toMatchObject({ view: "skills", dock: "skills" });
  dock.remove();
});
