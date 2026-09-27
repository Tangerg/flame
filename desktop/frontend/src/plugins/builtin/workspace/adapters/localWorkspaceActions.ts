import type { ClientHost } from "@/platform/host";
import { configureLocalWorkspace } from "../application/ports/localWorkspace";

function absoluteWorkspacePath(cwd: string, path: string): string {
  return path.startsWith("/") ? path : `${cwd.replace(/\/+$/, "")}/${path}`;
}

export function installLocalWorkspaceActions(
  host: Pick<ClientHost, "openPath" | "revealPath">,
  canAccessLocalWorkspace: () => boolean,
): () => void {
  return configureLocalWorkspace({
    available: canAccessLocalWorkspace,
    open(cwd, path) {
      if (!canAccessLocalWorkspace()) return Promise.resolve(false);
      return host.openPath(absoluteWorkspacePath(cwd, path));
    },
    reveal(cwd, path) {
      if (!canAccessLocalWorkspace()) return Promise.resolve(false);
      return host.revealPath(absoluteWorkspacePath(cwd, path));
    },
  });
}
