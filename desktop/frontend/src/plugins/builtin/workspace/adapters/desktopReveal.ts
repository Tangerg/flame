import { getContainer } from "@/main/container";

function absoluteWorkspacePath(cwd: string, path: string): string {
  return path.startsWith("/") ? path : `${cwd.replace(/\/+$/, "")}/${path}`;
}

export function revealWorkspacePath(cwd: string, path: string): Promise<boolean> {
  return getContainer().desktop.revealPath(absoluteWorkspacePath(cwd, path));
}

export function openWorkspacePath(cwd: string, path: string): Promise<boolean> {
  return getContainer().desktop.openPath(absoluteWorkspacePath(cwd, path));
}
