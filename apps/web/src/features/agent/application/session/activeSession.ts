import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import {
  type AgentSessionSummary,
  useAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";

import { type AgentOpenSessions } from "@/features/agent/application/session/sessionAccess";
export type { AgentOpenSessions } from "@/features/agent/application/session/sessionSelectionModel";

export type ActiveSessionWorkspaceSelection =
  { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string };

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

export function useObserveActiveSession(): void {
  useAgentDependencies().useObserveActiveSession();
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

export function openExistingAgentSession(
  agentAccess: AgentDependencies,
  id: string,
  signal: AbortSignal,
) {
  return agentAccess.state.selectExistingSession(
    id,
    signal,
    async (reading) => (await agentAccess.runtime.loadSessionSummary(id, reading)) !== null,
  );
}

export function closeActiveAgentSession(agentAccess: AgentDependencies): boolean {
  const id = getActiveSessionId(agentAccess);
  if (!id) return false;
  agentAccess.state.closeSession(id);
  return true;
}

export function useActiveSession(): AgentSessionSummary | undefined {
  return useAgentSessionSummary(useActiveSessionId() || undefined).data ?? undefined;
}

export function useActiveSessionWorkspace(): ActiveSessionWorkspaceSelection {
  const sessionId = useActiveSessionId();
  const { data: session } = useAgentSessionSummary(sessionId || undefined);
  if (!sessionId) return { status: "ready" };
  return session
    ? { status: "ready", cwd: session.workspace.path }
    : { status: "resolving", sessionId };
}
