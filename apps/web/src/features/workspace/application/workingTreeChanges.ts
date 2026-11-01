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

const NO_CHANGES: ReadonlyMap<string, WorkspaceFileChange> = new Map();

export function useWorkingTreeFiles(): ReadonlyMap<string, WorkspaceFileChange> {
  const data = useChangedFiles();
  return useMemo(
    () => (data ? new Map(data.map((file) => [file.path, file])) : NO_CHANGES),
    [data],
  );
}
