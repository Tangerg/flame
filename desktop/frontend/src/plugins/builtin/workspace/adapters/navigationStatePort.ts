// Which surface fills the content card, and which settings pane is open, are
// the user's location — they come from the Navigator, so history holds them.
// What the dock has OPEN, and the per-view state inside it, is memory the
// location doesn't describe: that stays in the store.
import { useShellLayoutStore } from "./shellLayoutStore";
import { WORKSPACE_DOCK_CATALOG } from "../application/navigation";
import { useContextDockStore } from "./contextDockStore";
import { navigator } from "@/lib/navigation";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

/** Leaving a promoted view returns to the chat, which is the only other surface. */
function selectChat(): void {
  navigator().go({ view: null });
}

/** The dock is open exactly when the location names a destination. */
function dockSnapshot() {
  return {
    open: navigator().get().dock !== null,
    viewIds: useContextDockStore.getState().dockViewIds,
    activeViewId: navigator().get().dock,
  };
}

function showDockView(id: string, alsoLeavePromotedView: boolean): void {
  // Remembered by the mover rather than by a subscriber on the location: this
  // port is installed while plugins load, before the router exists.
  useContextDockStore.getState().rememberDockView(id);
  navigator().go(alsoLeavePromotedView ? { view: null, dock: id } : { dock: id });
}

export function installWorkspaceNavigationPort(): () => void {
  return configureWorkspaceNavigationPort({
    useActiveViewId: () => navigator().use((location) => location.view),
    useDock: () => ({
      open: navigator().use((location) => location.dock !== null),
      viewIds: useContextDockStore((state) => state.dockViewIds),
      activeViewId: navigator().use((location) => location.dock),
    }),
    useFileFocus: () => useContextDockStore((state) => state.fileFocus),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useSettingsPaneTarget: () => navigator().use((location) => location.settings),
    useExpandedToolIds: () => useContextDockStore((state) => state.expandedToolIds),
    useSelectedToolId: () => useContextDockStore((state) => state.selectedToolId),
    useSelectTool: () => useContextDockStore((state) => state.setSelectedToolId),
    useToggleTool: () => useContextDockStore((state) => state.toggleExpandedTool),
    // The drawer follows the user's preference and nothing else. The dock is a
    // separate resizable column, so opening it cannot override that preference.
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
    // Taking the whole card leaves the dock's own selection alone: closing the
    // full view brings back whatever the user had beside the chat.
    openView: (id) => navigator().go({ view: id }),
    // One move: the tab opens and the location shows it, leaving the promoted
    // view behind.
    openViewInDock: (id) => {
      useContextDockStore.getState().openDockTab(id);
      showDockView(id, true);
    },
    selectDockView: (id) => {
      if (useContextDockStore.getState().dockViewIds.includes(id)) showDockView(id, false);
    },
    closeDockView: (id) => {
      const next = useContextDockStore.getState().closeDockTab(id);
      if (navigator().get().dock !== id) return;
      if (next === null) navigator().go({ dock: null });
      else showDockView(next, false);
    },
    closeOtherDockViews: (id) => {
      const state = useContextDockStore.getState();
      if (!state.dockViewIds.includes(id)) return;
      state.closeOtherDockTabs(id);
      showDockView(id, false);
    },
    closeAllDockViews: () => {
      useContextDockStore.getState().closeAllDockTabs();
      navigator().go({ dock: null });
    },
    reorderDockView: (id, toIndex) => useContextDockStore.getState().reorderDockTab(id, toIndex),
    collapseDock: () => navigator().go({ dock: null }),
    showDock: (defaultViewId) => {
      const target = useContextDockStore.getState().dockTabToShow(defaultViewId);
      // The catalogue is a destination, not a tab: opening one for it would leave the person
      // closing a tab they never asked for.
      if (target !== WORKSPACE_DOCK_CATALOG) useContextDockStore.getState().openDockTab(target);
      showDockView(target, true);
    },
    /** A stale id is a no-op: it is not the surface on screen. */
    closeView: (id) => {
      if (navigator().get().view === id) selectChat();
    },
    activeViewId: () => navigator().get().view,
    dock: dockSnapshot,
    setSettingsPane: (pane) => navigator().go({ settings: pane }),
    focusFile: (path) => useContextDockStore.getState().focusFile(path),
    openFile: (path, line) => {
      useContextDockStore.getState().setFileViewer(path, line);
      useContextDockStore.getState().openDockTab("file");
      showDockView("file", true);
    },
    selectedToolId: () => useContextDockStore.getState().selectedToolId,
    setSelectedTool: (id) => useContextDockStore.getState().setSelectedToolId(id),
    locateTool: (id) => {
      selectChat();
      useContextDockStore.getState().revealTool(id);
      if (!focusConversationTool(id) && typeof requestAnimationFrame === "function") {
        requestAnimationFrame(() => focusConversationTool(id));
      }
    },
    // A fresh renderer or a same-session Host rebind adopts the URL: location
    // already owns whether the dock survived open or collapsed. A real session
    // move restores that session's memory with `replace`, because this is the
    // tail of the move the user already made — going back should leave the
    // session, not undo its dock.
    activateSessionScope: (sessionId) => {
      const state = useContextDockStore.getState();
      const adoptsCurrentLocation =
        state.activeSessionScopeId === null || state.activeSessionScopeId === sessionId;
      const remembered = state.activateSessionScope(sessionId);
      if (adoptsCurrentLocation) {
        const located = navigator().get().dock;
        if (located !== null) state.adoptDockLocation(located);
        return;
      }
      if (navigator().get().dock !== remembered) {
        navigator().go({ dock: remembered }, { replace: true });
      }
    },
    forgetSessionScopes: (openSessionIds) =>
      useContextDockStore.getState().forgetSessionScopes(openSessionIds),
  });
}

function focusConversationTool(itemId: string): boolean {
  const anchor = document.getElementById(itemId);
  if (!anchor) return false;
  anchor.scrollIntoView?.({ block: "center" });
  anchor.querySelector<HTMLElement>("button")?.focus({ preventScroll: true });
  return true;
}
