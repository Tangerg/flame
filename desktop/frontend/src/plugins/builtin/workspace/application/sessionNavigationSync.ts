import type { AgentOpenSessions } from "@/plugins/builtin/agent/public/session";

export type AgentSessionListener = (sessionId: string) => void;
export type AgentSessionLifecycleListener = (state: AgentOpenSessions) => void;

export interface WorkspaceSessionNavigationPorts {
  activeSessionId: () => string;
  lifecycleSnapshot: () => AgentOpenSessions;
  subscribeActiveSessionId: (listener: AgentSessionListener) => () => void;
  subscribeLifecycle: (listener: AgentSessionLifecycleListener) => () => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

export function syncWorkspaceSessionLifecycle(
  state: AgentOpenSessions,
  ports: Pick<WorkspaceSessionNavigationPorts, "forgetSessionScopes">,
): void {
  ports.forgetSessionScopes(state.openSessionIds);
}

/**
 * Keep the dock's per-session memory pointed at the session the user is in.
 * Navigation itself owns clearing a promoted view; lifecycle synchronization
 * only activates the current scope and forgets scopes for closed sessions.
 */
export function bindWorkspaceSessionNavigation(ports: WorkspaceSessionNavigationPorts): () => void {
  ports.activateSessionScope(ports.activeSessionId());
  ports.forgetSessionScopes(ports.lifecycleSnapshot().openSessionIds);

  const unsubscribeSession = ports.subscribeActiveSessionId((sessionId) => {
    ports.activateSessionScope(sessionId);
  });
  const unsubscribeLifecycle = ports.subscribeLifecycle((state) => {
    syncWorkspaceSessionLifecycle(state, ports);
  });

  return () => {
    unsubscribeSession();
    unsubscribeLifecycle();
  };
}
