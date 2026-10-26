import { type AgentAccess } from "@/features/agent/public/agent";
import { queryClient, replaceCachedRead } from "@/lib/queryClient";
import { AGENT_SESSIONS_KEY } from "@/features/agent/public/session";
import { synchronizeMountedAgentSessions } from "@/features/agent/public/session";
import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY } from "@/features/agent/public/approvalPolicy";
import { HOOKS_KEY } from "@/features/hooks/public/queries";
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/features/providers/public/queries";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  MCP_EXPOSURE_KEY,
} from "@/features/mcp/public/serverCatalog";
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
  hooks: HOOKS_KEY,
  models: MODELS_KEY,
  mcpServers: MCP_SERVERS_KEY,
  mcpTools: MCP_TOOLS_KEY,
  mcpExposure: MCP_EXPOSURE_KEY,
  providers: PROVIDERS_KEY,
  sessions: AGENT_SESSIONS_KEY,
  utilityRole: UTILITY_ROLE_KEY,
  embeddingRole: EMBEDDING_ROLE_KEY,
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
    synchronizeMountedAgentSessions(agentAccess, {
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
      void replaceCachedRead({ queryKey: agentAccess.usage.queryKey() });
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
  synchronizeMountedAgentSessions(agentAccess, { ownership: "retire-live" });
  void queryClient.cancelQueries({ predicate: workspaceReadModels(agentAccess) });
}

export function replaceWorkspaceServerScope(agentAccess: AgentAccess): void {
  synchronizeMountedAgentSessions(agentAccess, { ownership: "replace-server" });
  void queryClient.resetQueries({ predicate: workspaceReadModels(agentAccess) });
}

function replaceWorkspaceReadModels(agentAccess: AgentAccess): void {
  synchronizeMountedAgentSessions(agentAccess, { ownership: "replace-live" });
  void replaceCachedRead({ predicate: workspaceReadModels(agentAccess) });
}

function workspaceReadModels(agentAccess: AgentAccess) {
  return (query: { queryKey: readonly unknown[] }) =>
    query.queryKey[0] !== agentAccess.usage.queryKey()[0] || agentAccess.usage.ownsQuery(query);
}
