import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

export function forkSessionAt(id: string, fromRunId?: string): Promise<void> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
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
  return useCallback((id) => forkSessionAt(id), []);
}
