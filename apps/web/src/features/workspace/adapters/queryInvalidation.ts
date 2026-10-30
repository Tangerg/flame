import { type AgentAccess } from "@/features/agent/public/agent";
import { queryClient, replaceCachedRead } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";

import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY } from "@/features/agent/public/approvalPolicy";
import { MODELS_KEY, PROVIDERS_KEY } from "@/features/providers/public/queries";
import {
  WORKSPACE_FILES_CHANGED_KEY,
  WORKSPACE_LIST_FILES_KEY,
  WORKSPACE_READ_FILE_KEY,
} from "@/features/workspace/public/queries";
import {
  workspaceInvalidations,
  workspaceQueryAffected,
  type WorkspaceEventLike,
  type WorkspaceInvalidationTarget,
} from "@/features/workspace/domain/eventInvalidation";
const QUERY_KEYS: Record<
  Exclude<WorkspaceInvalidationTarget, "all" | "agentSessionProjection" | "sessionUsage">,
  string
> = {
  approvalMode: APPROVAL_MODE_KEY,
  approvalRules: APPROVAL_RULES_KEY,
  fileList: WORKSPACE_LIST_FILES_KEY,
  fileRead: WORKSPACE_READ_FILE_KEY,
  filesChanged: WORKSPACE_FILES_CHANGED_KEY,
  models: MODELS_KEY,
  providers: PROVIDERS_KEY,
  sessions: AGENT_SESSIONS_KEY,
};

function invalidateWorkspaceTargets(
  agentAccess: AgentAccess,
  targets: WorkspaceInvalidationTarget[],
  event: WorkspaceEventLike,
): void {
  if (targets.includes("all")) {
    replaceWorkspaceReadModels(agentAccess);
    return;
  }
  if (targets.includes("agentSessionProjection")) {
    agentAccess.synchronizeMountedAgentSessions({
      sessionIds: event.sessionIds,
      ownership: "after-live",
    });
  }
  for (const target of targets) {
    if (target === "all") continue;
    if (target === "agentSessionProjection") {
      continue;
    }
    if (target === "sessionUsage") {
      void replaceCachedRead({ queryKey: agentAccess.sessionUsageQueryKey() });
      continue;
    }
    const predicate = (query: { queryKey: readonly unknown[] }) =>
      workspaceQueryAffected(
        target,
        query.queryKey[1] as { cwd?: string; path?: string } | undefined,
        event,
      );
    void replaceCachedRead({ queryKey: [QUERY_KEYS[target]], predicate });
  }
}

export function invalidateWorkspaceEvent(agentAccess: AgentAccess, ev: WorkspaceEventLike): void {
  invalidateWorkspaceTargets(agentAccess, workspaceInvalidations(ev), ev);
}

export function invalidateWorkspaceEverything(agentAccess: AgentAccess): void {
  replaceWorkspaceReadModels(agentAccess);
}

export function retireWorkspaceReadModels(agentAccess: AgentAccess): void {
  agentAccess.synchronizeMountedAgentSessions({ ownership: "retire-live" });
  void queryClient.cancelQueries({ predicate: workspaceReadModels(agentAccess) });
}

export function replaceWorkspaceServerScope(agentAccess: AgentAccess): void {
  agentAccess.synchronizeMountedAgentSessions({ ownership: "replace-server" });
  void queryClient.resetQueries({ predicate: workspaceReadModels(agentAccess) });
}

function replaceWorkspaceReadModels(agentAccess: AgentAccess): void {
  agentAccess.synchronizeMountedAgentSessions({ ownership: "replace-live" });
  void replaceCachedRead({ predicate: workspaceReadModels(agentAccess) });
}

function workspaceReadModels(agentAccess: AgentAccess) {
  return (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] !== agentAccess.sessionUsageQueryKey()[0] ||
    agentAccess.ownsSessionUsageQuery(query);
}
