import {
  isWorkspaceResolving,
  resolvedWorkspaceCwd,
  useActiveSessionWorkspace,
} from "@/features/agent/public/session";
import { useRuntimeCapability } from "@/features/connection/public/capabilities";
import { useMemo } from "react";
import {
  type WorkspaceFileChange,
  useWorkspaceFileChanges,
} from "@/features/workspace/application/workspaceQueries";
export function useChangedFiles(): WorkspaceFileChange[] | undefined {
  const gitEnabled = useRuntimeCapability("git");
  const workspace = useActiveSessionWorkspace();
  const cwd = resolvedWorkspaceCwd(workspace);
  return useWorkspaceFileChanges(
    gitEnabled && !isWorkspaceResolving(workspace) ? { cwd } : undefined,
  ).data;
}

export interface WorkingTree {
  files: ReadonlyMap<string, WorkspaceFileChange>;
  // Folders holding a change at any depth, so a collapsed folder can still say so.
  folders: ReadonlySet<string>;
}

const CLEAN: WorkingTree = { files: new Map(), folders: new Set() };

function folderPaths(path: string): string[] {
  const parts = path.split("/");
  return parts.slice(1).map((_, index) => parts.slice(0, index + 1).join("/"));
}

export function useWorkingTree(): WorkingTree {
  const data = useChangedFiles();
  return useMemo(() => {
    if (!data) return CLEAN;
    const folders = new Set(
      data.flatMap((file) => [file.path, file.previousPath ?? ""].flatMap(folderPaths)),
    );
    return { files: new Map(data.map((file) => [file.path, file])), folders };
  }, [data]);
}
