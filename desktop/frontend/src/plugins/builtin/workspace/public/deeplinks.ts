import {
  openWorkspaceDiffForFile,
  openWorkspaceView,
  openWorkspaceClientDiagnostics,
} from "../application/navigation";

export function openDiagnosticsView(): void {
  openWorkspaceClientDiagnostics();
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
