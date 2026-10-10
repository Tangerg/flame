import {
  openWorkspaceDiffForFile,
  openWorkspaceView,
  openWorkspaceViewInDock,
} from "../application/navigation";

export function openDiagnosticsView(): void {
  openWorkspaceViewInDock("diagnostics");
}

export function openSettingsView(): void {
  openWorkspaceView("settings");
}

export function openDiffViewInDock(): void {
  openWorkspaceDiffForFile("");
}

export function openFileInWorkingTreeDiff(path: string): void {
  openWorkspaceDiffForFile(path);
}
