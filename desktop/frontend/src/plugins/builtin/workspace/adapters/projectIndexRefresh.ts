import {
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "@/plugins/builtin/agent/public/session";
import { WORKSPACE_PROJECTS_KEY } from "@/plugins/builtin/workspace/public/queries";
import { replaceCachedRead } from "./queryInvalidation";

/**
 * Owns the cross-context edge: the agent PUBLISHES Session facts and workspace invalidates
 * its own named query, so neither takes a reverse dependency.
 *
 * Query-cache lifecycle events are deliberately NOT the signal: the project read depends
 * only on Session identity, cwd and updated time, so status changes and observer churn
 * must not refetch it.
 */
export function installProjectIndexRefresh(): () => void {
  return subscribeAgentSessionProjection(workspaceProjectRevision, () => {
    replaceCachedRead({ queryKey: [WORKSPACE_PROJECTS_KEY] });
  });
}

export function workspaceProjectRevision(
  sessions: readonly AgentSessionSummary[] | undefined,
): string {
  return JSON.stringify(
    sessions?.map(({ id, workspace, time }) => [id, workspace.path, time]) ?? null,
  );
}
