import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceCapability } from "./workspaceCapabilities";
import { useMemo } from "react";
import { type WorkspaceFileChange, useWorkspaceFileChanges } from "./workspaceQueries";

export interface WorkingTreeChanges {
  files: number;
  added: number;
  removed: number;
}

function useChangedFiles(): WorkspaceFileChange[] | undefined {
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  return useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
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

export function useWorkingTreeChanges(): WorkingTreeChanges | null {
  const data = useChangedFiles();
  if (!data || data.length === 0) return null;
  return data.reduce(
    (sum, file) => ({
      files: sum.files,
      added: sum.added + (file.added ?? 0),
      removed: sum.removed + (file.removed ?? 0),
    }),
    { files: data.length, added: 0, removed: 0 },
  );
}
