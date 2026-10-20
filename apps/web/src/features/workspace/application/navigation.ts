import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import {
  workspaceNavigation,
  type WorkspaceColumnWidth,
  type WorkspaceDrawer,
  type WorkspaceOptionalColumnWidth,
  type WorkspaceDockSnapshot,
  type WorkspaceFileFocusSnapshot,
  type WorkspaceFileViewer,
  type WorkspaceInspection,
  type WorkspaceSourcePresentation,
  type WorkspaceViewMemory,
} from "@/features/workspace/application/ports/navigationState";
export const WORKSPACE_DOCK_CATALOG = "catalog";

export const WORKSPACE_SETTINGS_VIEW = "settings";

export function useActiveWorkspaceViewId(): string | null {
  return workspaceNavigation().useActiveViewId();
}

export function useWorkspaceDock(): WorkspaceDockSnapshot {
  return workspaceNavigation().useDock();
}

export function useWorkspaceInspection(): WorkspaceInspection | null {
  return workspaceNavigation().useInspection();
}

export function returnFromWorkspaceInspection(
  inspection: WorkspaceInspection,
  presentation?: WorkspaceSourcePresentation,
): boolean {
  return workspaceNavigation().returnFromInspection(inspection, presentation);
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

export function openWorkspaceViewInDock(id: string, origin?: HTMLElement): void {
  workspaceNavigation().openViewInDock(id, origin);
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

export function openWorkspaceDiffForFile(path: string, origin?: HTMLElement): void {
  workspaceNavigation().focusFile(path);
  const views = lookupExtensionPoint(WORKSPACE_VIEW).filter((view) => view.material === "diff");
  workspaceNavigation().openViewInDock(
    views.length === 1 ? views[0]!.id : WORKSPACE_DOCK_CATALOG,
    origin,
  );
}

export function openWorkspaceFile(path: string, line?: number, origin?: HTMLElement): void {
  workspaceNavigation().openFile(path, line, origin);
}

export function closeWorkspaceFile(): void {
  workspaceNavigation().closeFile();
}

export function adoptWorkspaceSessionScope(sessionId: string): void {
  workspaceNavigation().adoptSessionScope(sessionId);
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

export function openWorkspaceSubagentRun(runId: string | null, origin?: HTMLElement): void {
  workspaceNavigation().openSubagentRun(runId, origin);
}

export function openWorkspaceClientDiagnostics(): void {
  const views = lookupExtensionPoint(WORKSPACE_VIEW).filter(
    (view) => view.material === "diagnostics",
  );
  workspaceNavigation().openViewInDock(views.length === 1 ? views[0]!.id : WORKSPACE_DOCK_CATALOG);
}
