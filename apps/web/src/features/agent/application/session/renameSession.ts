import { useAgent } from "@/features/agent/public/agent";
import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function useRenameSession(): (
  id: string,
  expectedRevision: number,
  title: string,
) => Promise<void> {
  const agentAccess = useAgent();

  return useCallback(
    async (id, expectedRevision, title) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      try {
        await writeAgentSessionSummary(owner, id, expectedRevision, (revision) =>
          runtime.updateSession({ sessionId: id, expectedRevision: revision, title }),
        );
      } catch (err) {
        if (!owner.isCurrent()) return;
        void invalidateAgentSessions();
        reportSessionError("rename", err);
      }
    },
    [agentAccess],
  );
}
