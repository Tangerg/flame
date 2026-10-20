import { useCallback } from "react";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";
import { agentRuntime } from "@/features/agent/application/ports/runtimeGateway";
import { agentSessionState } from "@/features/agent/application/ports/sessionState";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { agentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    const state = agentSessionState();
    try {
      await owner.settle(runtime.deleteSession(id));
      owner.assertCurrent();
      state.deleteSession(id);
      void invalidateAgentSessions();
    } catch (err) {
      if (owner.isCurrent()) reportSessionError("delete", err);
    }
  }, []);
}
