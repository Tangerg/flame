import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { openWorkspaceView } from "@/features/workspace/application/navigation";

export { openWorkspaceClientDiagnostics } from "@/features/workspace/application/navigation";

export function openSettingsView(workspaceAccess: WorkspaceAccess): void {
  openWorkspaceView(workspaceAccess, "settings");
}
