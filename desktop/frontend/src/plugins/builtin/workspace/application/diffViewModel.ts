import type { WorkspaceDiffMode } from "./diffVocabulary";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileFocus } from "@/plugins/builtin/workspace/public/navigation";
import { isVcsUnavailable } from "./vcsAvailability";
import type { WorkspaceDiff, WorkspaceFileDiff } from "./workspaceQueries";
import { useWorkspaceDiff } from "./workspaceQueries";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export type { WorkspaceFileDiff } from "./workspaceQueries";

interface WorkspaceDiffSubtext {
  added: number;
  removed: number;
  fileCount: number;
}

export interface WorkspaceDiffViewModel {
  baseline?: WorkspaceDiff["baseline"];
  files?: WorkspaceFileDiff[];
  subtext?: WorkspaceDiffSubtext;
  truncated: boolean;
}

export interface WorkspaceDiffFileHeader {
  path: string;
  previousPath?: string;
  added?: number;
  removed?: number;
  status: WorkspaceFileDiff["status"];
}

export function useWorkspaceDiffView(mode: WorkspaceDiffMode) {
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const fileFocus = useWorkspaceFileFocus();
  const query = useWorkspaceDiff(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd, mode } : undefined,
  );
  const view = workspaceDiffViewModel(query.data);
  return {
    fileFocus,
    data: query.data,
    files: view.files,
    isLoading: query.isLoading || workspace.status === "resolving",
    isError: query.isError,
    error: query.error,
    gitEnabled,
    notARepo: isVcsUnavailable(query.error),
    retry: () => void query.refetch(),
    view,
  };
}

export function workspaceDiffViewModel(data: WorkspaceDiff | undefined): WorkspaceDiffViewModel {
  const files = data?.files;
  if (!files) return { truncated: false };

  let added = 0;
  let removed = 0;
  for (const file of files) {
    added += file.added ?? 0;
    removed += file.removed ?? 0;
  }

  return {
    baseline: data.baseline,
    files,
    subtext: {
      added,
      removed,
      fileCount: files.length,
    },
    truncated: data.truncated ?? false,
  };
}

export function workspaceDiffFileHeader(file: WorkspaceFileDiff): WorkspaceDiffFileHeader {
  return {
    path: file.path,
    ...(file.previousPath ? { previousPath: file.previousPath } : {}),
    added: file.added,
    removed: file.removed,
    status: file.status,
  };
}
