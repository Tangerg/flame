import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { rpcErrorText } from "@/lib/rpcErrors";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function useRelocateSession(): (
  id: string,
  expectedRevision: number,
  cwd: string,
) => Promise<boolean> {
  const agentAccess = useAgentDependencies();

  return useCallback(
    async (id, expectedRevision, cwd) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      try {
        await writeAgentSessionSummary(owner, id, expectedRevision, (revision) =>
          runtime.updateSession({ sessionId: id, expectedRevision: revision, cwd }),
        );
        owner.assertCurrent();
        return true;
      } catch (err) {
        if (!owner.isCurrent()) return false;
        void invalidateAgentSessions();
        reportSessionError("relocate", err, rpcErrorText(err) ?? String(err));
        return false;
      }
    },
    [agentAccess],
  );
}
