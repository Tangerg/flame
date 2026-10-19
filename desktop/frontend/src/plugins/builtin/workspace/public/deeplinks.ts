import { openWorkspaceView, openWorkspaceClientDiagnostics } from "../application/navigation";

export function openDiagnosticsView(): void {
  openWorkspaceClientDiagnostics();
}

export function openSettingsView(): void {
  openWorkspaceView("settings");
}
