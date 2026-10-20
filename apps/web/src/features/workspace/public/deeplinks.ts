import {
  openWorkspaceView,
  openWorkspaceClientDiagnostics,
} from "@/features/workspace/application/navigation";
export function openDiagnosticsView(): void {
  openWorkspaceClientDiagnostics();
}

export function openSettingsView(): void {
  openWorkspaceView("settings");
}
