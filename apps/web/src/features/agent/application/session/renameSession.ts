import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";

import { rpcErrorText } from "@/lib/rpcErrors";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function useRenameSession(): (
  id: string,
  expectedRevision: number,
  title: string,
) => Promise<boolean> {
  const agentAccess = useAgentDependencies();

  return useCallback(
    async (id, expectedRevision, title) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      try {
        await writeAgentSessionSummary(owner, id, () =>
          runtime.updateSession({ sessionId: id, expectedRevision, title }),
        );
        return true;
      } catch (err) {
        if (!owner.isCurrent()) return false;
        void invalidateAgentSessions();
        reportSessionError("rename", err, rpcErrorText(err));
        return false;
      }
    },
    [agentAccess],
  );
}
