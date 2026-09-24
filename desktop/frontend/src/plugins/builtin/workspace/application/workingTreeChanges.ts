import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceCapability } from "./workspaceCapabilities";
import { useWorkspaceFileChanges } from "./workspaceQueries";

export interface WorkingTreeChanges {
  files: number;
  added: number;
  removed: number;
}

export function useWorkingTreeChanges(): WorkingTreeChanges | null {
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const { data } = useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
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
