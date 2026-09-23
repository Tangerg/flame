import type { AgentSessionSummary } from "./sessionQueries";
import { AGENT_SESSIONS_KEY } from "./sessionQueries";
import { queryClient } from "@/lib/queryClient";
import { agentSessionState } from "../ports/sessionState";
import { selectAgentSession, getActiveSessionId } from "./activeSession";

export type SessionStep = 1 | -1;

export function stepAgentSession(
  sessions: readonly AgentSessionSummary[],
  activeId: string,
  step: SessionStep,
): string | undefined {
  if (sessions.length === 0) return undefined;
  const at = sessions.findIndex((session) => session.id === activeId);
  if (at < 0) return (step === 1 ? sessions[0] : sessions[sessions.length - 1])?.id;
  const next = (at + step + sessions.length) % sessions.length;
  return sessions[next]?.id;
}

export function stepActiveAgentSession(step: SessionStep): void {
  const state = agentSessionState();
  const sessions = (
    queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]) ?? []
  ).filter((session) => !state.isDraftSession(session.id));
  const next = stepAgentSession(sessions, getActiveSessionId(), step);
  if (next !== undefined && next !== getActiveSessionId()) selectAgentSession(next);
}
