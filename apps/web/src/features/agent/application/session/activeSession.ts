import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentSessionSummary } from "@/features/agent/application/session/sessionQueries";
import { useAgentSessions } from "@/features/agent/application/session/sessionQueries";

import { type AgentOpenSessions } from "@/features/agent/application/session/sessionAccess";
export type { AgentOpenSessions } from "@/features/agent/application/session/sessionSelectionModel";

export type ActiveSessionWorkspaceSelection =
  { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string };

// resolvedWorkspaceCwd and isWorkspaceResolving own the two facts every consumer
// derives from a selection: the directory to act in, and whether the active
// session's workspace is still being located. A resolving selection has no cwd.
export function resolvedWorkspaceCwd(
  selection: ActiveSessionWorkspaceSelection,
): string | undefined {
  return selection.status === "ready" ? selection.cwd : undefined;
}

export function isWorkspaceResolving(selection: ActiveSessionWorkspaceSelection): boolean {
  return selection.status === "resolving";
}

export function useActiveSessionId(): string {
  const agentAccess = useAgentDependencies();

  return agentAccess.state.useActiveSessionId();
}

export function getActiveSessionId(agentAccess: AgentDependencies): string {
  return agentAccess.state.getActiveSessionId();
}

export function getAgentSessionLifecycleSnapshot(
  agentAccess: AgentDependencies,
): AgentOpenSessions {
  return agentAccess.state.getLifecycleSnapshot();
}

export function subscribeActiveSessionId(
  agentAccess: AgentDependencies,
  onChange: (sessionId: string) => void,
): () => void {
  return agentAccess.state.subscribeActiveSessionId(onChange);
}

export function subscribeAgentSessionLifecycle(
  agentAccess: AgentDependencies,
  onChange: (snapshot: AgentOpenSessions) => void,
): () => void {
  return agentAccess.state.subscribeLifecycle(onChange);
}

export function subscribeDeletedAgentSession(
  agentAccess: AgentDependencies,
  onDeleted: (sessionId: string) => void,
): () => void {
  return agentAccess.state.subscribeDeletedSession(onDeleted);
}

export function selectAgentSession(agentAccess: AgentDependencies, id: string): void {
  agentAccess.state.selectSession(id);
}

export function closeActiveAgentSession(agentAccess: AgentDependencies): boolean {
  const id = getActiveSessionId(agentAccess);
  if (!id) return false;
  agentAccess.state.closeSession(id);
  return true;
}

export function useActiveSession(): AgentSessionSummary | undefined {
  const activeSessionId = useActiveSessionId();
  const { data } = useAgentSessions();
  if (!activeSessionId) return undefined;
  return data?.find((s) => s.id === activeSessionId);
}

export function activeSessionWorkspaceSelection(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): ActiveSessionWorkspaceSelection {
  if (!activeSessionId) return { status: "ready" };
  const session = sessions?.find((candidate) => candidate.id === activeSessionId);
  return session
    ? { status: "ready", cwd: session.workspace.path }
    : { status: "resolving", sessionId: activeSessionId };
}

export function useActiveSessionWorkspace(): ActiveSessionWorkspaceSelection {
  const activeSessionId = useActiveSessionId();
  const { data } = useAgentSessions();
  return activeSessionWorkspaceSelection(activeSessionId, data);
}
