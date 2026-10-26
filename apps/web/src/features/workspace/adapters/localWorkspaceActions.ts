import { type ClientHost } from "@/platform/host";
import { type LocalWorkspace } from "@/features/workspace/application/localWorkspace";
function absoluteWorkspacePath(cwd: string, path: string): string {
  return path.startsWith("/") ? path : `${cwd.replace(/\/+$/, "")}/${path}`;
}

export function createLocalWorkspaceActions(
  host: Pick<ClientHost, "openPath" | "revealPath">,
  canAccessLocalWorkspace: () => boolean,
): LocalWorkspace {
  return {
    available: canAccessLocalWorkspace,
    open(cwd, path) {
      if (!canAccessLocalWorkspace()) return Promise.resolve(false);
      return host.openPath(absoluteWorkspacePath(cwd, path));
    },
    reveal(cwd, path) {
      if (!canAccessLocalWorkspace()) return Promise.resolve(false);
      return host.revealPath(absoluteWorkspacePath(cwd, path));
    },
  };
}
