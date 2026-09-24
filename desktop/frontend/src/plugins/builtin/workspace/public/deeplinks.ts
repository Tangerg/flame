import {
  openWorkspaceDiffForFile,
  openWorkspaceView,
  openWorkspaceViewInDock,
} from "../application/navigation";

export function openTimelineView(): void {
  openWorkspaceViewInDock("timeline");
}

export function openDiagnosticsView(): void {
  openWorkspaceViewInDock("diagnostics");
}

export function openSettingsView(): void {
  openWorkspaceView("settings");
}

export function openDiffViewInDock(): void {
  openWorkspaceViewInDock("diff");
}

export function openFileInWorkingTreeDiff(path: string): void {
  openWorkspaceDiffForFile(path);
}
