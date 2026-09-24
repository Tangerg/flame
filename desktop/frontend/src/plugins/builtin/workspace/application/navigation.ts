import {
  workspaceNavigation,
  type WorkspaceColumnWidth,
  type WorkspaceDrawer,
  type WorkspaceOptionalColumnWidth,
  type WorkspaceDockSnapshot,
  type WorkspaceFileFocusSnapshot,
  type WorkspaceFileViewer,
  type WorkspaceViewMemory,
} from "./ports/navigationState";

export const WORKSPACE_DOCK_CATALOG = "catalog";

export const WORKSPACE_SETTINGS_VIEW = "settings";

export function useActiveWorkspaceViewId(): string | null {
  return workspaceNavigation().useActiveViewId();
}

export function useWorkspaceDock(): WorkspaceDockSnapshot {
  return workspaceNavigation().useDock();
}

export function useWorkspaceFileFocus(): WorkspaceFileFocusSnapshot {
  return workspaceNavigation().useFileFocus();
}

export function useWorkspaceViewMemory(): WorkspaceViewMemory {
  return workspaceNavigation().useViewMemory();
}

export function rememberWorkspaceView(change: Partial<WorkspaceViewMemory>): void {
  workspaceNavigation().remember(change);
}

export function useWorkspaceFileViewer(): WorkspaceFileViewer | null {
  return workspaceNavigation().useFileViewer();
}

export function useWorkspaceSettingsPaneTarget(): string | null {
  return workspaceNavigation().useSettingsPaneTarget();
}

export function useExpandedWorkspaceToolIds(): Set<string> {
  return workspaceNavigation().useExpandedToolIds();
}

export function useToggleWorkspaceTool(): (id: string) => void {
  return workspaceNavigation().useToggleTool();
}

export function useSidebarDrawer(): WorkspaceDrawer {
  return workspaceNavigation().useSidebarDrawer();
}

export function useSidebarWidth(): WorkspaceColumnWidth {
  return workspaceNavigation().useSidebarWidth();
}

export function useDockWidth(): WorkspaceOptionalColumnWidth {
  return workspaceNavigation().useDockWidth();
}

export function toggleWorkspaceSidebar(): void {
  workspaceNavigation().toggleSidebar();
}

export function selectWorkspaceChat(): void {
  workspaceNavigation().selectChat();
}

export function openWorkspaceView(id: string): void {
  workspaceNavigation().openView(id);
}

export function openWorkspaceViewInDock(id: string): void {
  workspaceNavigation().openViewInDock(id);
}

export function selectWorkspaceDockView(id: string): void {
  workspaceNavigation().selectDockView(id);
}

export function closeWorkspaceDockView(id: string): void {
  workspaceNavigation().closeDockView(id);
}

export function closeOtherWorkspaceDockViews(id: string): void {
  workspaceNavigation().closeOtherDockViews(id);
}

export function closeAllWorkspaceDockViews(): void {
  workspaceNavigation().closeAllDockViews();
}

export function reorderWorkspaceDockView(id: string, toIndex: number): void {
  workspaceNavigation().reorderDockView(id, toIndex);
}

export function closeActiveWorkspaceDockView(): boolean {
  const activeViewId = workspaceNavigation().dock().activeViewId;
  if (!activeViewId) return false;
  if (activeViewId === WORKSPACE_DOCK_CATALOG) workspaceNavigation().collapseDock();
  else workspaceNavigation().closeDockView(activeViewId);
  return true;
}

export function collapseWorkspaceDock(): void {
  workspaceNavigation().collapseDock();
}

export function showWorkspaceDock(): void {
  workspaceNavigation().showDock(WORKSPACE_DOCK_CATALOG);
}

export function toggleWorkspaceDock(): void {
  if (workspaceNavigation().dock().open) {
    workspaceNavigation().collapseDock();
  } else {
    workspaceNavigation().showDock(WORKSPACE_DOCK_CATALOG);
  }
}

export function closeWorkspaceView(id: string): void {
  workspaceNavigation().closeView(id);
}

export function closeActiveWorkspaceView(): boolean {
  const activeViewId = workspaceNavigation().activeViewId();
  if (!activeViewId) return false;
  workspaceNavigation().closeView(activeViewId);
  return true;
}

export function openWorkspaceSettingsPane(pane: string): void {
  workspaceNavigation().setSettingsPane(pane);
  workspaceNavigation().openView(WORKSPACE_SETTINGS_VIEW);
}

export function openWorkspaceDiffForFile(path: string): void {
  workspaceNavigation().focusFile(path);
  workspaceNavigation().openViewInDock("diff");
}

export function openWorkspaceFile(path: string, line?: number): void {
  workspaceNavigation().openFile(path, line);
}

export function closeWorkspaceFile(): void {
  workspaceNavigation().closeFile();
}

export function locateWorkspaceTool(id: string): void {
  workspaceNavigation().locateTool(id);
}

export function activateWorkspaceSessionScope(sessionId: string): void {
  workspaceNavigation().activateSessionScope(sessionId);
}

export function forgetWorkspaceSessionScopes(openSessionIds: string[]): void {
  workspaceNavigation().forgetSessionScopes(openSessionIds);
}

export function useWorkspaceSubagentRunId(): string | null {
  return workspaceNavigation().useSubagentRunId();
}

export function openWorkspaceSubagentRun(runId: string | null): void {
  workspaceNavigation().openSubagentRun(runId);
}
