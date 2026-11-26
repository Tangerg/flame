import { type ShellLayoutStore } from "@/features/workspace/adapters/shellLayoutStore";
import {
  resolveInspectionReturnDock,
  WORKSPACE_DOCK_CATALOG,
  WORKSPACE_SETTINGS_VIEW,
} from "@/features/workspace/application/navigation";
import { type ContextDockStore } from "@/features/workspace/adapters/contextDockStore";
import { type AppLocation, type Navigator } from "@/lib/navigation";
import {
  type WorkspaceNavigationPort,
  type WorkspaceChatPresentation,
} from "@/features/workspace/application/navigationState";
// The Dock view an element renders in, so an inspection opened from it returns there.
export function dockViewMarker(viewId: string | undefined): { "data-dock-view-id"?: string } {
  return viewId === undefined ? {} : { "data-dock-view-id": viewId };
}

function markedDockViewId(element: HTMLElement): string | null {
  return element.closest<HTMLElement>("[data-dock-view-id]")?.dataset.dockViewId ?? null;
}

interface SettingsFocusRegistration {
  capture: () => (() => void) | undefined;
  prepareReturn?: () => void;
}

export function createWorkspaceNavigation(
  navigation: Navigator,
  dockStore: ContextDockStore,
  useShellLayoutStore: ShellLayoutStore,
  endpoint: () => string,
): WorkspaceNavigationPort {
  const useContextDockStore = dockStore.store;
  const activateContextDockStorage = dockStore.activate;
  let settingsFocus: SettingsFocusRegistration | undefined;
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
    const returnDockId = markedDockViewId(origin);
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

  function showDockView(
    id: string,
    arrival: DockArrival,
    origin?: HTMLElement,
    target?: string,
  ): void {
    inspectFrom(id, origin);
    useContextDockStore.getState().adoptDockLocation(id);
    navigation.go(
      arrival === "alone"
        ? { view: null, dock: id, ...(target === undefined ? {} : { target }) }
        : { dock: id },
    );
  }

  const shownTarget = (id: string, location: AppLocation) =>
    (location.view ?? location.dock) === id ? location.target : null;

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
    registerSettingsFocusCapture: (capture) => {
      const registration: SettingsFocusRegistration = { capture };
      settingsFocus = registration;
      const unsubscribe = navigation.subscribe((location, previous) => {
        if (location.view !== WORKSPACE_SETTINGS_VIEW || location.session !== previous.session)
          registration.prepareReturn = undefined;
      });
      return () => {
        unsubscribe();
        if (settingsFocus === registration) settingsFocus = undefined;
      };
    },
    useExpandedToolIds: () => useContextDockStore((state) => state.expandedToolIds),
    useSetToolExpanded: () => useContextDockStore((state) => state.setToolExpanded),
    useSidebarDrawer: () => ({
      collapsed: useShellLayoutStore((state) => state.sidebarCollapsed),
      toggle: useShellLayoutStore((state) => state.toggleSidebar),
    }),
    useSidebarLayout: () => ({
      layout: useShellLayoutStore((state) => state.sidebarLayout),
      save: useShellLayoutStore((state) => state.saveSidebarLayout),
    }),
    useDockLayout: () => ({
      layout: useShellLayoutStore((state) => state.dockLayout),
      save: useShellLayoutStore((state) => state.saveDockLayout),
    }),
    toggleSidebar: () => useShellLayoutStore.getState().toggleSidebar(),
    selectChat,
    openView: (id, origin) => {
      if (settingsFocus && id === WORKSPACE_SETTINGS_VIEW && navigation.get().view !== id)
        settingsFocus.prepareReturn = settingsFocus.capture();
      inspectFrom(id, origin);
      navigation.go({ view: id });
    },
    openViewInDock: (id, origin, target) => showDockView(id, "alone", origin, target),
    subscribeViewTarget: (id, listener) => {
      const current = shownTarget(id, navigation.get());
      if (current !== null) listener(current);
      return navigation.subscribe((location, previous) => {
        const target = shownTarget(id, location);
        if (target !== null && target !== shownTarget(id, previous)) listener(target);
      });
    },
    viewHref: (id, target) =>
      navigation.href({ session: "", view: id, dock: null, target: target ?? null }),
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
    showDock: (defaultViewId, presentation = "dock", origin) => {
      const target =
        (presentation === "main" ? navigation.get().dock : null) ??
        useContextDockStore.getState().dockTabToShow(defaultViewId);
      if (presentation === "main") {
        inspectFrom(target, origin);
        navigation.go({ view: target });
      } else showDockView(target, "alone", origin);
    },
    closeView: (id) => {
      if (navigation.get().view !== id) return;
      if (id === WORKSPACE_SETTINGS_VIEW && settingsFocus) {
        const prepareReturn = settingsFocus.prepareReturn;
        settingsFocus.prepareReturn = undefined;
        prepareReturn?.();
      }
      selectChat();
    },
    activeViewId: () => navigation.get().view,
    dock: dockSnapshot,
    setSettingsPane: (pane) => navigation.go({ settings: pane }),
    focusFile: (path) => useContextDockStore.getState().focusFile(path),
    openFile: (path, line, origin) => {
      inspectFrom("file", origin);
      const viewer = { path, line: line ?? 0 };
      useContextDockStore.getState().setFileViewer(viewer);
      if (origin || navigation.get().view !== "file") showDockView("file", "alone");
      return viewer;
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
