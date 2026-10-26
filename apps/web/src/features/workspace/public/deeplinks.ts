import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import {
  openWorkspaceView,
  openWorkspaceClientDiagnostics,
} from "@/features/workspace/application/navigation";
export function openDiagnosticsView(workspaceAccess: WorkspaceAccess): void {
  openWorkspaceClientDiagnostics(workspaceAccess);
}

export function openSettingsView(workspaceAccess: WorkspaceAccess): void {
  openWorkspaceView(workspaceAccess, "settings");
}
