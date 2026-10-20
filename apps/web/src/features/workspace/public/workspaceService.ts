import { service } from "dougong";
export interface WorkspaceService {
  openView(id: string): void;
  closeView(id: string): void;
}

export const WORKSPACE = service<WorkspaceService>("flame.shell.workspace");
