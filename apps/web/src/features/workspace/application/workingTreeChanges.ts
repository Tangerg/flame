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
export interface WorkingTreeChanges {
  files: number;
  added: number | undefined;
  removed: number | undefined;
}

function useChangedFiles(): WorkspaceFileChange[] | undefined {
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

export function summarizeWorkingTreeChanges(
  data: readonly WorkspaceFileChange[] | undefined,
): WorkingTreeChanges | null {
  if (!data || data.length === 0) return null;
  return data.reduce<WorkingTreeChanges>(
    (sum, file) => ({
      files: sum.files,
      added:
        sum.added === undefined || file.added === undefined ? undefined : sum.added + file.added,
      removed:
        sum.removed === undefined || file.removed === undefined
          ? undefined
          : sum.removed + file.removed,
    }),
    { files: data.length, added: 0, removed: 0 },
  );
}

export function useWorkingTreeChanges(): WorkingTreeChanges | null {
  return summarizeWorkingTreeChanges(useChangedFiles());
}
