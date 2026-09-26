import { getContainer } from "@/main/container";

export function localWorkspaceActionsAvailable(): boolean {
  return getContainer().localWorkspaceAvailable();
}

function absoluteWorkspacePath(cwd: string, path: string): string {
  return path.startsWith("/") ? path : `${cwd.replace(/\/+$/, "")}/${path}`;
}

export function revealWorkspacePath(cwd: string, path: string): Promise<boolean> {
  if (!localWorkspaceActionsAvailable()) return Promise.resolve(false);
  return getContainer().host.revealPath(absoluteWorkspacePath(cwd, path));
}

export function openWorkspacePath(cwd: string, path: string): Promise<boolean> {
  if (!localWorkspaceActionsAvailable()) return Promise.resolve(false);
  return getContainer().host.openPath(absoluteWorkspacePath(cwd, path));
}
