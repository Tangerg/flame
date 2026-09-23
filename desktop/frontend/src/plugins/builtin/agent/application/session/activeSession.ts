import { discardAbandonedDraft } from "./discardAbandonedDraft";
import type { AgentSessionSummary } from "./sessionQueries";
import { useAgentSessions } from "./sessionQueries";
import { agentSessionState, type AgentOpenSessions } from "../ports/sessionState";

export type { AgentOpenSessions } from "../ports/sessionState";

export type ActiveSessionWorkspaceSelection =
  { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string };

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

export function selectAgentSession(id: string): void {
  agentSessionState().selectSession(id);
}

export function closeActiveAgentSession(): boolean {
  const id = getActiveSessionId();
  if (!id) return false;
  discardAbandonedDraft(id);
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
