import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";

import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

export function useToggleFavorite(): (
  id: string,
  expectedRevision: number,
  favorite: boolean,
) => Promise<void> {
  const agentAccess = useAgentDependencies();

  return useCallback(
    async (id, expectedRevision, favorite) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      try {
        await writeAgentSessionSummary(owner, id, expectedRevision, (revision) =>
          runtime.updateSession({ sessionId: id, expectedRevision: revision, favorite }),
        );
      } catch (err) {
        if (!owner.isCurrent()) return;
        void invalidateAgentSessions();
        reportSessionError("favorite", err);
      }
    },
    [agentAccess],
  );
}
