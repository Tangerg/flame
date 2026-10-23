import { type AgentSessionSummary } from "@/features/agent/application/session/sessionQueries";
import { useAgentSessions } from "@/features/agent/application/session/sessionQueries";
import { agentSessionState } from "@/features/agent/application/ports/sessionState";
import { type AgentOpenSessions } from "@/features/agent/application/ports/sessionState";
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
  return agentSessionState().useActiveSessionId();
}

export function getActiveSessionId(): string {
  return agentSessionState().getActiveSessionId();
}

export function getAgentSessionLifecycleSnapshot(): AgentOpenSessions {
  return agentSessionState().getLifecycleSnapshot();
}

export function subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void {
  return agentSessionState().subscribeActiveSessionId(onChange);
}

export function subscribeAgentSessionLifecycle(
  onChange: (snapshot: AgentOpenSessions) => void,
): () => void {
  return agentSessionState().subscribeLifecycle(onChange);
}

export function subscribeDeletedAgentSession(onDeleted: (sessionId: string) => void): () => void {
  return agentSessionState().subscribeDeletedSession(onDeleted);
}

export function selectAgentSession(id: string): void {
  agentSessionState().selectSession(id);
}

export function closeActiveAgentSession(): boolean {
  const id = getActiveSessionId();
  if (!id) return false;
  agentSessionState().closeSession(id);
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
