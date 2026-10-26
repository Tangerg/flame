import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import {
  openWorkspaceView,
  openWorkspaceClientDiagnostics,
} from "@/features/workspace/application/navigation";
export function openDiagnosticsView(workspaceAccess: WorkspaceAccess, origin?: HTMLElement): void {
  openWorkspaceClientDiagnostics(workspaceAccess, origin);
}

export function openSettingsView(workspaceAccess: WorkspaceAccess): void {
  openWorkspaceView(workspaceAccess, "settings");
}
