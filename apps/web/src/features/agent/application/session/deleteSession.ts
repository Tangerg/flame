import { useAgent } from "@/features/agent/public/agent";
import { useCallback } from "react";
import { invalidateAgentSessions } from "@/features/agent/application/session/sessionQueries";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function useDeleteSession(): (id: string) => Promise<void> {
  const agentAccess = useAgent();

  return useCallback(
    async (id) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      const state = agentAccess.state;
      try {
        await owner.settle(runtime.deleteSession(id));
        owner.assertCurrent();
        state.deleteSession(id);
        void invalidateAgentSessions();
      } catch (err) {
        if (owner.isCurrent()) reportSessionError("delete", err);
      }
    },
    [agentAccess],
  );
}
