import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { contributeForTest } from "@/test/kernel";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { beforeEach, describe, expect, it } from "vitest";
import { WorkspaceFileFocus } from "@/features/workspace/adapters/contextDockStore";

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
  selectWorkspaceChat,
  selectWorkspaceDockView,
  showWorkspaceDock,
  toggleWorkspaceDock,
  openWorkspaceDiffForFile,
  openWorkspaceClientDiagnostics,
} from "@/features/workspace/application/navigation";
function reset() {
  document.body.replaceChildren();
  testClient.navigation.go({ view: "v2", settings: null });
  testClient.workspace.dock.store.setState({
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
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    openWorkspaceViewInDock(testClient.workspace.access, "skills");

    expect(testClient.navigation.get().view).toBeNull();
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      dockViewIds: ["skills", "diff"],
      lastViewId: "skills",
    });
  });

  it("only selects views that already belong to the dock", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    selectWorkspaceDockView(testClient.workspace.access, "diff");
    expect(testClient.navigation.get().dock).toBe("skills");
  });

  it("a full view leaves the dock workspace alone", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    collapseWorkspaceDock(testClient.workspace.access);
    openWorkspaceView(testClient.workspace.access, "v3");

    expect(testClient.navigation.get().view).toBe("v3");
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      dockViewIds: ["skills"],
      lastViewId: "skills",
    });
  });

  it("closing a full view returns to chat without changing dock tabs", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceView(testClient.workspace.access, "v3");

    expect(closeActiveWorkspaceView(testClient.workspace.access)).toBe(true);
    expect(testClient.navigation.get().view).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["skills"]);
  });

  it("preserves the existing Dock when selecting the conversation by default", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceView(testClient.workspace.access, "settings");

    selectWorkspaceChat(testClient.workspace.access);

    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "skills" });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["skills"]);
  });

  it("reveals the conversation alone in one history step while retaining its Dock tabs", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceView(testClient.workspace.access, "settings");
    const previous = testClient.navigation.get();

    selectWorkspaceChat(testClient.workspace.access, "hide-dock");

    expect(testClient.navigation.get()).toMatchObject({ view: null, dock: null });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["skills"]);
    testClient.navigation.back();
    expect(testClient.navigation.get()).toEqual(previous);
  });

  it("collapse and show are a lossless round trip", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    collapseWorkspaceDock(testClient.workspace.access);
    showWorkspaceDock(testClient.workspace.access);

    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("opens the remembered Dock material full width after collapse", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    collapseWorkspaceDock(testClient.workspace.access);

    showWorkspaceDock(testClient.workspace.access, "main");

    expect(testClient.navigation.get()).toMatchObject({ view: "diff", dock: null });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["skills", "diff"]);
  });

  it("keeps the actively selected catalog when opening it full width", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceViewInDock(testClient.workspace.access, WORKSPACE_DOCK_CATALOG);

    showWorkspaceDock(testClient.workspace.access, "main");

    expect(testClient.navigation.get()).toMatchObject({
      view: WORKSPACE_DOCK_CATALOG,
      dock: WORKSPACE_DOCK_CATALOG,
    });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["skills"]);
  });

  it("offers the catalog when an empty collapsed Dock opens full width", () => {
    collapseWorkspaceDock(testClient.workspace.access);

    showWorkspaceDock(testClient.workspace.access, "main");

    expect(testClient.navigation.get()).toMatchObject({ view: WORKSPACE_DOCK_CATALOG, dock: null });
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);
  });

  it("showing an empty dock offers the catalogue rather than choosing a panel", () => {
    showWorkspaceDock(testClient.workspace.access);

    expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      dockViewIds: [],
      lastViewId: WORKSPACE_DOCK_CATALOG,
    });
  });

  it("returns an emptied dock to its catalogue rather than closing it", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "diff");

    closeWorkspaceDockView(testClient.workspace.access, "diff");

    expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  });

  it("returns to the catalogue when every tab is closed at once", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    openWorkspaceViewInDock(testClient.workspace.access, "plan");

    closeAllWorkspaceDockViews(testClient.workspace.access);

    expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  });

  it("restores the catalogue after renderer replacement without opening a phantom tab", () => {
    testClient.navigation.go({ session: "s1", dock: WORKSPACE_DOCK_CATALOG });

    adoptWorkspaceSessionScope(testClient.workspace.access, "s1");

    expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);
  });

  it("does not resurrect the last closed tab when returning to a session", () => {
    activateWorkspaceSessionScope(testClient.workspace.access, "s1");
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    closeWorkspaceDockView(testClient.workspace.access, "diff");

    testClient.navigation.go({ session: "s2" });
    activateWorkspaceSessionScope(testClient.workspace.access, "s2");
    testClient.navigation.go({ session: "s1" });
    activateWorkspaceSessionScope(testClient.workspace.access, "s1");

    expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);
  });

  it("closes an empty dock through the active-view close command", () => {
    showWorkspaceDock(testClient.workspace.access);

    expect(closeActiveWorkspaceDockView(testClient.workspace.access)).toBe(true);

    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual([]);
  });

  it("shows the remembered panel instead of the catalogue when there is one", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "diff");
    collapseWorkspaceDock(testClient.workspace.access);

    showWorkspaceDock(testClient.workspace.access);

    expect(testClient.navigation.get().dock).toBe("diff");
  });

  it("toggles the visible dock without discarding its session tabs", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "diff");

    toggleWorkspaceDock(testClient.workspace.access);
    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["diff"]);

    toggleWorkspaceDock(testClient.workspace.access);
    expect(testClient.navigation.get().dock).toBe("diff");
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["diff"]);
  });

  it("does not reopen a collapsed dock when the same session scope is rebound", () => {
    testClient.navigation.go({ session: "s1", dock: null });
    testClient.workspace.dock.store.setState({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope(testClient.workspace.access, "s1");

    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.workspace.dock.store.getState().dockViewIds).toEqual(["diff"]);
  });

  it("distinguishes a real move from the initialized sessionless scope", () => {
    testClient.navigation.go({ session: "s1", dock: "diff" });
    testClient.workspace.dock.store.setState({
      activeSessionScopeId: "",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope(testClient.workspace.access, "s1");

    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: [],
      lastViewId: null,
    });
  });

  it("closes the active dock tab before the session-level command can run", () => {
    openWorkspaceViewInDock(testClient.workspace.access, "skills");
    openWorkspaceViewInDock(testClient.workspace.access, "diff");

    expect(closeActiveWorkspaceDockView(testClient.workspace.access)).toBe(true);
    expect(testClient.workspace.dock.store.getState()).toMatchObject({
      dockViewIds: ["skills"],
      lastViewId: "skills",
    });
  });
});

it("keeps subagent selection in session-scoped history without replacing the main conversation", () => {
  testClient.navigation.go({ session: "parent-session" });
  openWorkspaceSubagentRun(testClient.workspace.access, "child-run");
  expect(testClient.navigation.get()).toMatchObject({
    session: "parent-session",
    view: null,
    dock: "subagents",
    subagent: "child-run",
  });
  openWorkspaceSubagentRun(testClient.workspace.access, "nested-run");
  testClient.navigation.back();
  expect(testClient.navigation.get().subagent).toBe("child-run");
  testClient.navigation.go({ session: "other-session" });
  expect(testClient.navigation.get().subagent).toBeNull();
  testClient.navigation.back();
  expect(testClient.navigation.get()).toMatchObject({
    session: "parent-session",
    subagent: "child-run",
  });
});

it("keeps subagent selection and the way back in the active full view", () => {
  testClient.navigation.go({
    session: "parent-session",
    view: "subagents",
    dock: "diff",
    subagent: null,
  });

  openWorkspaceSubagentRun(testClient.workspace.access, "child-run");
  expect(testClient.navigation.get()).toMatchObject({
    session: "parent-session",
    view: "subagents",
    dock: "diff",
    subagent: "child-run",
  });

  openWorkspaceSubagentRun(testClient.workspace.access, "nested-run");
  testClient.navigation.back();
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: "child-run",
  });

  openWorkspaceSubagentRun(testClient.workspace.access, null);
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: null,
  });
});

it("captures a nested task source in explicit main and ends only that local trail", () => {
  testClient.navigation.go({ session: "parent-session", view: null, dock: null });
  const outerSource = document.createElement("button");
  document.body.append(outerSource);
  openWorkspaceSubagentRun(testClient.workspace.access, "child", outerSource);
  const outer = testClient.workspace.dock.store.getState().inspection!;
  testClient.navigation.go({ view: "subagents", dock: "diff" });
  const reader = document.createElement("button");
  document.body.append(reader);

  openWorkspaceSubagentRun(testClient.workspace.access, "nested", reader);
  const fromChild = testClient.workspace.dock.store.getState().inspection!;
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: "nested",
  });
  expect(fromChild).toMatchObject({
    viewId: "subagents",
    returnViewId: "subagents",
    returnDockId: null,
    returnSubagentRunId: "child",
  });
  expect(fromChild.origin).toBe(reader);
  expect(fromChild.previous).toBe(outer);
  openWorkspaceSubagentRun(testClient.workspace.access, "deeper", reader);
  const fromNested = testClient.workspace.dock.store.getState().inspection!;
  expect(fromNested.returnSubagentRunId).toBe("nested");
  expect(fromNested.previous).toBe(fromChild);

  expect(returnFromWorkspaceInspection(testClient.workspace.access, fromNested)).toBe(true);
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: "nested",
  });
  expect(testClient.workspace.dock.store.getState().inspection).toBe(fromChild);

  openWorkspaceSubagentRun(testClient.workspace.access, null);
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: null,
  });
  expect(testClient.workspace.dock.store.getState().inspection).toBe(outer);
  openWorkspaceSubagentRun(testClient.workspace.access, "child");
  expect(testClient.navigation.get()).toMatchObject({
    view: "subagents",
    dock: "diff",
    subagent: "child",
  });
  expect(testClient.workspace.dock.store.getState().inspection).toBe(outer);
  outerSource.remove();
  reader.remove();
});

it("restores the file inspected before a task opened a different file", () => {
  testClient.navigation.go({ session: "session", view: null });
  openWorkspaceFile(testClient.workspace.access, "src/original.ts", 12);
  const file = document.createElement("div");
  file.dataset.dockViewId = "file";
  const source = document.createElement("button");
  file.append(source);
  document.body.append(file);
  openWorkspaceSubagentRun(testClient.workspace.access, "child", source);
  const fromFile = testClient.workspace.dock.store.getState().inspection!;
  const task = document.createElement("div");
  task.dataset.dockViewId = "subagents";
  const nested = document.createElement("button");
  task.append(nested);
  document.body.append(task);
  openWorkspaceFile(testClient.workspace.access, "src/evidence.ts", 40, nested);
  const fromTask = testClient.workspace.dock.store.getState().inspection!;
  expect(returnFromWorkspaceInspection(testClient.workspace.access, fromTask)).toBe(true);
  expect(testClient.navigation.get()).toMatchObject({ dock: "subagents", subagent: "child" });
  expect(returnFromWorkspaceInspection(testClient.workspace.access, fromFile)).toBe(true);
  expect(testClient.navigation.get()).toMatchObject({ dock: "file", subagent: "child" });
  expect(testClient.workspace.dock.store.getState().fileViewer).toEqual({
    path: "src/original.ts",
    line: 12,
  });
  expect(testClient.workspace.dock.store.getState().inspection).toBeNull();
  expect(returnFromWorkspaceInspection(testClient.workspace.access, fromTask)).toBe(false);
  file.remove();
  task.remove();
});

it("opens the contribution catalogue when Diff has no admitted renderer or several choices", async () => {
  openWorkspaceDiffForFile(testClient.workspace.access, "a.ts");
  expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
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
  openWorkspaceDiffForFile(testClient.workspace.access, "b.ts");
  expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
});

it("opens only a unique admitted client diagnostics contribution", async () => {
  openWorkspaceClientDiagnostics(testClient.workspace.access);
  expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
  await contributeForTest((ctx) =>
    ctx.contribute(WORKSPACE_VIEW, {
      id: "package:first:diagnostics",
      title: "Diagnostics",
      material: "diagnostics",
      dock: "workspace",
      component: () => null,
    }),
  );
  openWorkspaceClientDiagnostics(testClient.workspace.access);
  expect(testClient.navigation.get().dock).toBe("package:first:diagnostics");
  await contributeForTest((ctx) =>
    ctx.contribute(WORKSPACE_VIEW, {
      id: "package:second:diagnostics",
      title: "Other diagnostics",
      material: "diagnostics",
      dock: "workspace",
      component: () => null,
    }),
  );
  openWorkspaceClientDiagnostics(testClient.workspace.access);
  expect(testClient.navigation.get().dock).toBe(WORKSPACE_DOCK_CATALOG);
});

it("keeps a presented Files page while its tree and line navigation change the file", () => {
  testClient.navigation.go({ session: "session", view: "file", dock: "diff" });
  openWorkspaceFile(testClient.workspace.access, "src/tree.ts");
  openWorkspaceFile(testClient.workspace.access, "src/tree.ts", 24);
  expect(testClient.navigation.get()).toMatchObject({ view: "file", dock: "diff" });
  expect(testClient.workspace.dock.store.getState()).toMatchObject({
    fileViewer: { path: "src/tree.ts", line: 24 },
    inspection: null,
  });

  testClient.navigation.go({ view: "v3" });
  const source = document.createElement("button");
  document.body.append(source);
  openWorkspaceFile(testClient.workspace.access, "src/inspected.ts", 3, source);
  expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "file" });
  expect(testClient.workspace.dock.store.getState().inspection).toMatchObject({
    returnViewId: "v3",
  });
  source.remove();
});

it("restores an ordinary Dock source full width only when the return asks for it", () => {
  testClient.navigation.go({ session: "session", view: null });
  openWorkspaceViewInDock(testClient.workspace.access, "skills");
  const dock = document.createElement("div");
  dock.dataset.dockViewId = "skills";
  const source = document.createElement("button");
  dock.append(source);
  document.body.append(dock);

  openWorkspaceFile(testClient.workspace.access, "src/a.ts", 1, source);
  expect(
    returnFromWorkspaceInspection(
      testClient.workspace.access,
      testClient.workspace.dock.store.getState().inspection!,
    ),
  ).toBe(true);
  expect(testClient.navigation.get()).toMatchObject({ view: null, dock: "skills" });

  openWorkspaceFile(testClient.workspace.access, "src/a.ts", 1, source);
  const inspection = testClient.workspace.dock.store.getState().inspection!;
  expect(returnFromWorkspaceInspection(testClient.workspace.access, inspection, "main")).toBe(true);
  expect(testClient.navigation.get()).toMatchObject({ view: "skills", dock: "skills" });
  dock.remove();
});
