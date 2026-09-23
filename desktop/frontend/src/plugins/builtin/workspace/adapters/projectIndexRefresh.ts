import {
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { WORKSPACE_PROJECTS_KEY } from "@/plugins/builtin/workspace/public/queries";
import { replaceCachedRead } from "@/lib/queryClient";

export function installProjectIndexRefresh(): () => void {
  return subscribeAgentSessionProjection(workspaceProjectRevision, () => {
    void replaceCachedRead({ queryKey: [WORKSPACE_PROJECTS_KEY] });
  });
}

export function workspaceProjectRevision(
  sessions: readonly AgentSessionSummary[] | undefined,
): string {
  return JSON.stringify(
    sessions?.map(({ id, workspace, time }) => [id, workspace.path, time]) ?? null,
  );
}
