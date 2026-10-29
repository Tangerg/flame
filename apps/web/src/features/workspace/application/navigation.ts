import { useWorkspace, type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import {
  type WorkspaceChatPresentation,
  type WorkspaceColumnWidth,
  type WorkspaceDrawer,
  type WorkspaceOptionalColumnWidth,
  type WorkspaceDockSnapshot,
  type WorkspaceFileFocusSnapshot,
  type WorkspaceFileViewer,
  type WorkspaceInspection,
  type WorkspaceViewPresentation,
  type WorkspaceViewMemory,
} from "@/features/workspace/application/navigationState";
export const WORKSPACE_DOCK_CATALOG = "catalog";

export const WORKSPACE_SETTINGS_VIEW = "settings";

export function resolveInspectionReturnDock(
  inspection: Pick<WorkspaceInspection, "returnViewId" | "returnDockId">,
  dockViewIds: readonly string[],
): string | null {
  return !inspection.returnViewId &&
    inspection.returnDockId &&
    (inspection.returnDockId === WORKSPACE_DOCK_CATALOG ||
      dockViewIds.includes(inspection.returnDockId))
    ? inspection.returnDockId
    : null;
}

export function useActiveWorkspaceViewId(): string | null {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useActiveViewId();
}

export function useWorkspaceDock(): WorkspaceDockSnapshot {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useDock();
}

export function useWorkspaceInspection(): WorkspaceInspection | null {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useInspection();
}

export function returnFromWorkspaceInspection(
  workspaceAccess: WorkspaceAccess,
  inspection: WorkspaceInspection,
  presentation?: WorkspaceViewPresentation,
): boolean {
  return workspaceAccess.navigation.returnFromInspection(inspection, presentation);
}

export function useWorkspaceFileFocus(): WorkspaceFileFocusSnapshot {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useFileFocus();
}

export function useWorkspaceViewMemory(): WorkspaceViewMemory {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useViewMemory();
}

export function rememberWorkspaceView(
  workspaceAccess: WorkspaceAccess,
  change: Partial<WorkspaceViewMemory>,
): void {
  workspaceAccess.navigation.remember(change);
}

export function useWorkspaceFileViewer(): WorkspaceFileViewer | null {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useFileViewer();
}

export function useWorkspaceSettingsPaneTarget(): string | null {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useSettingsPaneTarget();
}

export function useExpandedWorkspaceToolIds(): Set<string> {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useExpandedToolIds();
}

export function useToggleWorkspaceTool(): (id: string) => void {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useToggleTool();
}

export function useSidebarDrawer(): WorkspaceDrawer {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useSidebarDrawer();
}

export function useSidebarWidth(): WorkspaceColumnWidth {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useSidebarWidth();
}

export function useDockWidth(): WorkspaceOptionalColumnWidth {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useDockWidth();
}

export function toggleWorkspaceSidebar(workspaceAccess: WorkspaceAccess): void {
  workspaceAccess.navigation.toggleSidebar();
}

export function selectWorkspaceChat(
  workspaceAccess: WorkspaceAccess,
  presentation?: WorkspaceChatPresentation,
): void {
  workspaceAccess.navigation.selectChat(presentation);
}

export function openWorkspaceView(
  workspaceAccess: WorkspaceAccess,
  id: string,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.openView(id, origin);
}

export function openWorkspaceViewInDock(
  workspaceAccess: WorkspaceAccess,
  id: string,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.openViewInDock(id, origin);
}

export function selectWorkspaceDockView(workspaceAccess: WorkspaceAccess, id: string): void {
  workspaceAccess.navigation.selectDockView(id);
}

export function closeWorkspaceDockView(workspaceAccess: WorkspaceAccess, id: string): void {
  workspaceAccess.navigation.closeDockView(id);
}

export function closeOtherWorkspaceDockViews(workspaceAccess: WorkspaceAccess, id: string): void {
  workspaceAccess.navigation.closeOtherDockViews(id);
}

export function closeAllWorkspaceDockViews(workspaceAccess: WorkspaceAccess): void {
  workspaceAccess.navigation.closeAllDockViews();
}

export function reorderWorkspaceDockView(
  workspaceAccess: WorkspaceAccess,
  id: string,
  toIndex: number,
): void {
  workspaceAccess.navigation.reorderDockView(id, toIndex);
}

export function closeActiveWorkspaceDockView(workspaceAccess: WorkspaceAccess): boolean {
  const activeViewId = workspaceAccess.navigation.dock().activeViewId;
  if (!activeViewId) return false;
  if (activeViewId === WORKSPACE_DOCK_CATALOG) workspaceAccess.navigation.collapseDock();
  else workspaceAccess.navigation.closeDockView(activeViewId);
  return true;
}

export function collapseWorkspaceDock(workspaceAccess: WorkspaceAccess): void {
  workspaceAccess.navigation.collapseDock();
}

export function showWorkspaceDock(
  workspaceAccess: WorkspaceAccess,
  presentation?: WorkspaceViewPresentation,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.showDock(WORKSPACE_DOCK_CATALOG, presentation, origin);
}

export function toggleWorkspaceDock(workspaceAccess: WorkspaceAccess): void {
  if (workspaceAccess.navigation.dock().open) {
    workspaceAccess.navigation.collapseDock();
  } else {
    workspaceAccess.navigation.showDock(WORKSPACE_DOCK_CATALOG);
  }
}

export function closeWorkspaceView(workspaceAccess: WorkspaceAccess, id: string): void {
  workspaceAccess.navigation.closeView(id);
}

export function closeActiveWorkspaceView(workspaceAccess: WorkspaceAccess): boolean {
  const activeViewId = workspaceAccess.navigation.activeViewId();
  if (!activeViewId) return false;
  workspaceAccess.navigation.closeView(activeViewId);
  return true;
}

export function openWorkspaceSettingsPane(workspaceAccess: WorkspaceAccess, pane: string): void {
  workspaceAccess.navigation.setSettingsPane(pane);
  workspaceAccess.navigation.openView(WORKSPACE_SETTINGS_VIEW);
}

export function openWorkspaceDiffForFile(
  workspaceAccess: WorkspaceAccess,
  path: string,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.focusFile(path);
  const views = lookupExtensionPoint(WORKSPACE_VIEW).filter((view) => view.material === "diff");
  workspaceAccess.navigation.openViewInDock(
    views.length === 1 ? views[0]!.id : WORKSPACE_DOCK_CATALOG,
    origin,
  );
}

export function openWorkspaceFile(
  workspaceAccess: WorkspaceAccess,
  path: string,
  line?: number,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.openFile(path, line, origin);
}

export function closeWorkspaceFile(workspaceAccess: WorkspaceAccess): void {
  workspaceAccess.navigation.closeFile();
}

export function adoptWorkspaceSessionScope(
  workspaceAccess: WorkspaceAccess,
  sessionId: string,
): void {
  workspaceAccess.navigation.adoptSessionScope(sessionId);
}

export function activateWorkspaceSessionScope(
  workspaceAccess: WorkspaceAccess,
  sessionId: string,
): void {
  workspaceAccess.navigation.activateSessionScope(sessionId);
}

export function forgetWorkspaceSessionScopes(
  workspaceAccess: WorkspaceAccess,
  openSessionIds: string[],
): void {
  workspaceAccess.navigation.forgetSessionScopes(openSessionIds);
}

export function useWorkspaceSubagentRunId(): string | null {
  const workspaceAccess = useWorkspace();

  return workspaceAccess.navigation.useSubagentRunId();
}

export function openWorkspaceSubagentRun(
  workspaceAccess: WorkspaceAccess,
  runId: string | null,
  origin?: HTMLElement,
): void {
  workspaceAccess.navigation.openSubagentRun(runId, origin);
}

export function openWorkspaceClientDiagnostics(
  workspaceAccess: WorkspaceAccess,
  origin?: HTMLElement,
): void {
  const views = lookupExtensionPoint(WORKSPACE_VIEW).filter(
    (view) => view.material === "diagnostics",
  );
  workspaceAccess.navigation.openViewInDock(
    views.length === 1 ? views[0]!.id : WORKSPACE_DOCK_CATALOG,
    origin,
  );
}
