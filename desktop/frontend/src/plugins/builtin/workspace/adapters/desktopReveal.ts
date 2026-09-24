import { getContainer } from "@/main/container";

export function revealWorkspacePath(cwd: string, path: string): Promise<boolean> {
  const absolute = path.startsWith("/") ? path : `${cwd.replace(/\/+$/, "")}/${path}`;
  return getContainer().desktop.revealPath(absolute);
}
