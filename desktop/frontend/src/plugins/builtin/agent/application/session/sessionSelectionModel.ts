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

export function reconcileOpenSessions(
  state: AgentOpenSessions & { provisionalSessionIds: Set<string> },
  liveIds: string[],
): AgentOpenSessions | null {
  const known = new Set([...liveIds, ...state.provisionalSessionIds]);
  const retainedOpenSessionIds = state.openSessionIds.filter((id) => known.has(id));
  const activeAlive = state.activeSessionId === "" || known.has(state.activeSessionId);
  const openSessionIds =
    activeAlive && state.activeSessionId !== ""
      ? openSession(retainedOpenSessionIds, state.activeSessionId)
      : retainedOpenSessionIds;
  const openSessionsChanged =
    openSessionIds.length !== state.openSessionIds.length ||
    openSessionIds.some((id, index) => id !== state.openSessionIds[index]);
  if (!openSessionsChanged && activeAlive) return null;
  return {
    openSessionIds,
    activeSessionId: activeAlive ? state.activeSessionId : (openSessionIds.at(-1) ?? ""),
  };
}

export function pruneDraftSessions(state: {
  openSessionIds: string[];
  draftSessionIds: Set<string>;
}): Set<string> | null {
  const live = new Set(state.openSessionIds);
  const draftSessionIds = new Set([...state.draftSessionIds].filter((id) => live.has(id)));
  return draftSessionIds.size === state.draftSessionIds.size ? null : draftSessionIds;
}
