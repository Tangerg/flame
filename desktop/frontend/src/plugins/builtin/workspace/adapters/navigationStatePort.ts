import { useShellLayoutStore } from "./shellLayoutStore";
import { WORKSPACE_DOCK_CATALOG } from "../application/navigation";
import { activateContextDockStorage, useContextDockStore } from "./contextDockStore";
import { navigator } from "@/lib/navigation";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

function selectChat(): void {
  navigator().go({ view: null });
}

function dockSnapshot() {
  return {
    open: navigator().get().dock !== null,
    viewIds: useContextDockStore.getState().dockViewIds,
    activeViewId: navigator().get().dock,
  };
}

type DockArrival = "alone" | "beside";

function showDockView(id: string, arrival: DockArrival, origin?: HTMLElement): void {
  if (origin) {
    useContextDockStore.setState({
      inspection: {
        viewId: id,
        origin,
        activeElement: origin.ownerDocument.activeElement,
        returnViewId: navigator().get().view,
        returnDockId:
          origin.closest<HTMLElement>("[data-dock-view-id]")?.dataset.dockViewId ?? null,
      },
    });
  }
  useContextDockStore.getState().adoptDockLocation(id);
  navigator().go(arrival === "alone" ? { view: null, dock: id } : { dock: id });
}

export function installWorkspaceNavigationPort(endpoint: () => string): () => void {
  activateContextDockStorage(endpoint());
  useContextDockStore.setState({ inspection: null });
  return configureWorkspaceNavigationPort({
    useActiveViewId: () => navigator().use((location) => location.view),
    useDock: () => ({
      open: navigator().use((location) => location.dock !== null),
      viewIds: useContextDockStore((state) => state.dockViewIds),
      activeViewId: navigator().use((location) => location.dock),
    }),
    useInspection: () => useContextDockStore((state) => state.inspection),
    returnFromInspection: (inspection) => {
      const state = useContextDockStore.getState();
      if (state.inspection !== inspection) return false;
      useContextDockStore.setState({ inspection: null });
      if (inspection.returnViewId) {
        navigator().go({ view: inspection.returnViewId });
      } else if (inspection.returnDockId && state.dockViewIds.includes(inspection.returnDockId)) {
        showDockView(inspection.returnDockId, "alone");
      } else {
        selectChat();
      }
      return true;
    },
    useSubagentRunId: () => navigator().use((location) => location.subagent),
    openSubagentRun: (runId) => {
      if (navigator().get().view === "subagents") {
        navigator().go({ subagent: runId });
        return;
      }
      useContextDockStore.getState().adoptDockLocation("subagents");
      navigator().go({ view: null, dock: "subagents", subagent: runId });
    },
    useFileFocus: () => useContextDockStore((state) => state.fileFocus),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useViewMemory: () => useContextDockStore((state) => state.memory),
    remember: (change) => useContextDockStore.getState().remember(change),
    useSettingsPaneTarget: () => navigator().use((location) => location.settings),
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
    openView: (id) => navigator().go({ view: id }),
    openViewInDock: (id, origin) => showDockView(id, "alone", origin),
    selectDockView: (id) => {
      if (useContextDockStore.getState().dockViewIds.includes(id)) showDockView(id, "beside");
    },
    closeDockView: (id) => {
      const next = useContextDockStore.getState().closeDockTab(id);
      if (navigator().get().dock !== id) return;
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
    collapseDock: () => navigator().go({ dock: null }),
    showDock: (defaultViewId) => {
      const target = useContextDockStore.getState().dockTabToShow(defaultViewId);
      showDockView(target, "alone");
    },
    closeView: (id) => {
      if (navigator().get().view === id) selectChat();
    },
    activeViewId: () => navigator().get().view,
    dock: dockSnapshot,
    setSettingsPane: (pane) => navigator().go({ settings: pane }),
    focusFile: (path) => useContextDockStore.getState().focusFile(path),
    openFile: (path, line, origin) => {
      useContextDockStore.getState().setFileViewer({ path, line: line ?? 0 });
      showDockView("file", "alone", origin);
    },
    closeFile: () => useContextDockStore.getState().setFileViewer(null),
    adoptSessionScope: (sessionId) => {
      activateContextDockStorage(endpoint());
      const state = useContextDockStore.getState();
      state.activateSessionScope(sessionId);
      const located = navigator().get().dock;
      if (located !== null) state.adoptDockLocation(located);
    },
    activateSessionScope: (sessionId) => {
      activateContextDockStorage(endpoint());
      const state = useContextDockStore.getState();
      if (state.activeSessionScopeId === sessionId) {
        const located = navigator().get().dock;
        if (located !== null) state.adoptDockLocation(located);
        return;
      }
      const remembered = state.activateSessionScope(sessionId);
      if (navigator().get().dock !== remembered) {
        navigator().go({ dock: remembered }, { replace: true });
      }
    },
    forgetSessionScopes: (openSessionIds) => {
      activateContextDockStorage(endpoint());
      useContextDockStore.getState().forgetSessionScopes(openSessionIds);
    },
  });
}
