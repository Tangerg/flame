import { type AgentAccess } from "@/features/agent/public/agent";
import { type AgentSessionSummary } from "@/features/agent/application/session/sessionQueries";
import { AGENT_SESSIONS_KEY } from "@/features/agent/application/session/sessionQueries";
import { queryClient } from "@/lib/queryClient";
import {
  selectAgentSession,
  getActiveSessionId,
} from "@/features/agent/application/session/activeSession";
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

export function stepActiveAgentSession(agentAccess: AgentAccess, step: SessionStep): void {
  const sessions = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]) ?? [];
  const next = stepAgentSession(sessions, getActiveSessionId(agentAccess), step);
  if (next !== undefined && next !== getActiveSessionId(agentAccess))
    selectAgentSession(agentAccess, next);
}
