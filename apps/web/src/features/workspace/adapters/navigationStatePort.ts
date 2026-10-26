import { type ShellLayoutStore } from "@/features/workspace/adapters/shellLayoutStore";
import {
  resolveInspectionReturnDock,
  WORKSPACE_DOCK_CATALOG,
} from "@/features/workspace/application/navigation";
import { type ContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { type Navigator } from "@/lib/navigation";
import {
  type WorkspaceNavigationPort,
  type WorkspaceChatPresentation,
} from "@/features/workspace/application/navigationState";
export function createWorkspaceNavigation(
  navigation: Navigator,
  dockStore: ContextDockStore,
  useShellLayoutStore: ShellLayoutStore,
  endpoint: () => string,
): WorkspaceNavigationPort {
  const useContextDockStore = dockStore.store;
  const activateContextDockStorage = dockStore.activate;
  function selectChat(presentation: WorkspaceChatPresentation = "preserve-dock"): void {
    navigation.go(presentation === "hide-dock" ? { view: null, dock: null } : { view: null });
  }

  function dockSnapshot() {
    return {
      open: navigation.get().dock !== null,
      viewIds: useContextDockStore.getState().dockViewIds,
      activeViewId: navigation.get().dock,
    };
  }

  type DockArrival = "alone" | "beside";

  function inspectFrom(id: string, origin?: HTMLElement): void {
    if (!origin) return;
    const location = navigation.get();
    const state = useContextDockStore.getState();
    const returnDockId =
      origin.closest<HTMLElement>("[data-dock-view-id]")?.dataset.dockViewId ?? null;
    const sourceViewId = location.view ?? (returnDockId === location.dock ? returnDockId : null);
    useContextDockStore.setState({
      inspection: {
        viewId: id,
        origin,
        activeElement: origin.ownerDocument.activeElement,
        returnViewId: location.view,
        returnDockId,
        returnSubagentRunId: location.subagent,
        returnFileViewer: state.fileViewer,
        previous:
          sourceViewId !== null && state.inspection?.viewId === sourceViewId
            ? state.inspection
            : null,
      },
    });
  }

  function showDockView(id: string, arrival: DockArrival, origin?: HTMLElement): void {
    inspectFrom(id, origin);
    useContextDockStore.getState().adoptDockLocation(id);
    navigation.go(arrival === "alone" ? { view: null, dock: id } : { dock: id });
  }

  return {
    useActiveViewId: () => navigation.use((location) => location.view),
    useDock: () => ({
      open: navigation.use((location) => location.dock !== null),
      viewIds: useContextDockStore((state) => state.dockViewIds),
      activeViewId: navigation.use((location) => location.dock),
    }),
    useInspection: () => useContextDockStore((state) => state.inspection),
    returnFromInspection: (inspection, presentation = "dock") => {
      const state = useContextDockStore.getState();
      if (state.inspection !== inspection) return false;
      useContextDockStore.setState({ inspection: inspection.previous });
      const returnDockId = resolveInspectionReturnDock(inspection, state.dockViewIds);
      if (inspection.returnViewId === "file" || inspection.returnDockId === "file") {
        state.setFileViewer(inspection.returnFileViewer);
      }
      if (returnDockId) state.adoptDockLocation(returnDockId);
      navigation.go({
        view: returnDockId && presentation === "main" ? returnDockId : inspection.returnViewId,
        ...(returnDockId ? { dock: returnDockId } : {}),
        ...(inspection.returnViewId === "subagents" || inspection.returnDockId === "subagents"
          ? { subagent: inspection.returnSubagentRunId }
          : {}),
      });
      return true;
    },
    useSubagentRunId: () => navigation.use((location) => location.subagent),
    openSubagentRun: (runId, origin) => {
      if (origin) {
        inspectFrom("subagents", origin);
      } else {
        let inspection = useContextDockStore.getState().inspection;
        while (
          inspection?.viewId === "subagents" &&
          (inspection.returnViewId ?? inspection.returnDockId) === "subagents"
        ) {
          inspection = inspection.previous;
        }
        useContextDockStore.setState({ inspection });
      }
      if (navigation.get().view === "subagents") {
        navigation.go({ subagent: runId });
        return;
      }
      useContextDockStore.getState().adoptDockLocation("subagents");
      navigation.go({ view: null, dock: "subagents", subagent: runId });
    },
    useFileFocus: () => useContextDockStore((state) => state.fileFocus),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useViewMemory: () => useContextDockStore((state) => state.memory),
    remember: (change) => useContextDockStore.getState().remember(change),
    useSettingsPaneTarget: () => navigation.use((location) => location.settings),
    useExpandedToolIds: () => useContextDockStore((state) => state.expandedToolIds),
    useToggleTool: () => useContextDockStore((state) => state.toggleExpandedTool),
    useSidebarDrawer: () => ({
      collapsed: useShellLayoutStore((state) => state.sidebarCollapsed),
      toggle: useShellLayoutStore((state) => state.toggleSidebar),
    }),
    useSidebarWidth: () => ({
      width: useShellLayoutStore((state) => state.sidebarWidth),
      setWidth: useShellLayoutStore((state) => state.setSidebarWidth),
    }),
    useDockWidth: () => {
      const setDockWidthRatio = useShellLayoutStore((state) => state.setDockWidthRatio);
      return {
        width: useShellLayoutStore((state) => state.dockWidthRatio),
        setWidth: setDockWidthRatio,
      };
    },
    toggleSidebar: () => useShellLayoutStore.getState().toggleSidebar(),
    selectChat,
    openView: (id) => navigation.go({ view: id }),
    openViewInDock: (id, origin) => showDockView(id, "alone", origin),
    selectDockView: (id) => {
      if (useContextDockStore.getState().dockViewIds.includes(id)) showDockView(id, "beside");
    },
    closeDockView: (id) => {
      const next = useContextDockStore.getState().closeDockTab(id);
      if (navigation.get().dock !== id) return;
      showDockView(next ?? WORKSPACE_DOCK_CATALOG, "beside");
    },
    closeOtherDockViews: (id) => {
      const state = useContextDockStore.getState();
      if (!state.dockViewIds.includes(id)) return;
      state.closeOtherDockTabs(id);
      showDockView(id, "beside");
    },
    closeAllDockViews: () => {
      useContextDockStore.getState().closeAllDockTabs();
      showDockView(WORKSPACE_DOCK_CATALOG, "beside");
    },
    reorderDockView: (id, toIndex) => useContextDockStore.getState().reorderDockTab(id, toIndex),
    collapseDock: () => navigation.go({ dock: null }),
    showDock: (defaultViewId, presentation = "dock") => {
      const target =
        (presentation === "main" ? navigation.get().dock : null) ??
        useContextDockStore.getState().dockTabToShow(defaultViewId);
      if (presentation === "main") navigation.go({ view: target });
      else showDockView(target, "alone");
    },
    closeView: (id) => {
      if (navigation.get().view === id) selectChat();
    },
    activeViewId: () => navigation.get().view,
    dock: dockSnapshot,
    setSettingsPane: (pane) => navigation.go({ settings: pane }),
    focusFile: (path) => useContextDockStore.getState().focusFile(path),
    openFile: (path, line, origin) => {
      inspectFrom("file", origin);
      useContextDockStore.getState().setFileViewer({ path, line: line ?? 0 });
      if (origin || navigation.get().view !== "file") showDockView("file", "alone");
    },
    closeFile: () => useContextDockStore.getState().setFileViewer(null),
    adoptSessionScope: (sessionId) => {
      activateContextDockStorage(endpoint());
      const state = useContextDockStore.getState();
      state.activateSessionScope(sessionId);
      const located = navigation.get().dock;
      if (located !== null) state.adoptDockLocation(located);
    },
    activateSessionScope: (sessionId) => {
      activateContextDockStorage(endpoint());
      const state = useContextDockStore.getState();
      if (state.activeSessionScopeId === sessionId) {
        const located = navigation.get().dock;
        if (located !== null) state.adoptDockLocation(located);
        return;
      }
      const remembered = state.activateSessionScope(sessionId);
      if (navigation.get().dock !== remembered) {
        navigation.go({ dock: remembered }, { replace: true });
      }
    },
    forgetSessionScopes: (openSessionIds) => {
      activateContextDockStorage(endpoint());
      useContextDockStore.getState().forgetSessionScopes(openSessionIds);
    },
  };
}
