export interface AgentOpenSessions {
  activeSessionId: string;
  openSessionIds: string[];
}

export function openSession(openSessionIds: string[], sessionId: string): string[] {
  return openSessionIds.includes(sessionId) ? openSessionIds : [...openSessionIds, sessionId];
}

export function closeOpenSession(state: AgentOpenSessions, sessionId: string): AgentOpenSessions {
  const index = state.openSessionIds.indexOf(sessionId);
  const openSessionIds = state.openSessionIds.filter((id) => id !== sessionId);
  const leavingActive = sessionId === state.activeSessionId;
  return {
    openSessionIds,
    activeSessionId: leavingActive
      ? (openSessionIds[index] ?? openSessionIds.at(-1) ?? "")
      : state.activeSessionId,
  };
}
