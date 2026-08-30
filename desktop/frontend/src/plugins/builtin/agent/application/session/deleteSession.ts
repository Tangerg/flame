import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

/** Closes the tab as well, reselecting a neighbour when the deleted session was active. */
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    const state = agentSessionState();
    try {
      // Session membership drives active/open navigation reconciliation. Keep
      // the query authoritative until Runtime commits the delete; treating a
      // local cache mutation as a server read can move the user even if the
      // command subsequently fails.
      await owner.settle(runtime.deleteSession(id));
      owner.assertCurrent();
      state.closeSession(id);
      void invalidateAgentSessions();
    } catch (err) {
      if (owner.isCurrent()) reportSessionError("delete", err);
    }
  }, []);
}
