import type { AgentSessionSummary } from "./sessionQueries";
import { AGENT_SESSIONS_KEY } from "./sessionQueries";
import { queryClient } from "@/lib/queryClient";
import { agentSessionState } from "../ports/sessionState";
import { selectAgentSession, getActiveSessionId } from "./activeSession";

export type SessionStep = 1 | -1;

/**
 * The session one step from the active one in the order the Work Index shows, wrapping at both
 * ends. Nothing selected — or a selection the list no longer carries, which a deletion leaves
 * behind — enters the list from whichever end the step came from.
 */
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
  const sessions = (queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]) ?? [])
    // A draft has no place in the order until it becomes a session, which is the same rule
    // the Work Index reads it by.
    .filter((session) => !state.isDraftSession(session.id));
  const next = stepAgentSession(sessions, getActiveSessionId(), step);
  if (next !== undefined && next !== getActiveSessionId()) selectAgentSession(next);
}
