import { type AgentAccess, useAgent } from "@/features/agent/public/agent";
import { useCallback } from "react";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function forkSessionAt(
  agentAccess: AgentAccess,
  id: string,
  fromRunId?: string,
): Promise<void> {
  const owner = agentAccess.commands.current();
  const runtime = agentAccess.runtime;
  const state = agentAccess.state;
  const key = fromRunId ? `${id}:${fromRunId}` : id;
  return owner
    .runSessionFork(key, async () => {
      const fork = await runtime.forkSession({ sessionId: id, fromRunId });
      owner.assertCurrent();
      state.selectSession(fork.id);
      void invalidateAgentSessions();
    })
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError("fork", error);
    });
}

export function useForkSession(): (id: string) => Promise<void> {
  const agentAccess = useAgent();

  return useCallback((id) => forkSessionAt(agentAccess, id), [agentAccess]);
}
