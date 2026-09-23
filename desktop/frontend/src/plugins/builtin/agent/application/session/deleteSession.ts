import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    const state = agentSessionState();
    try {
      await owner.settle(runtime.deleteSession(id));
      owner.assertCurrent();
      state.closeSession(id);
      void invalidateAgentSessions();
    } catch (err) {
      if (owner.isCurrent()) reportSessionError("delete", err);
    }
  }, []);
}
