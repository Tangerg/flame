import {
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/features/agent/public/session";
import { WORKSPACE_FOLDERS_KEY } from "@/features/workspace/public/queries";
import { replaceCachedRead } from "@/lib/queryClient";
export function installFolderIndexRefresh(): () => void {
  return subscribeAgentSessionProjection(workspaceFolderRevision, () => {
    void replaceCachedRead({ queryKey: [WORKSPACE_FOLDERS_KEY] });
  });
}

function workspaceFolderRevision(sessions: readonly AgentSessionSummary[] | undefined): string {
  return JSON.stringify(
    sessions?.map(({ id, workspace, time }) => [id, workspace.path, time]) ?? null,
  );
}
