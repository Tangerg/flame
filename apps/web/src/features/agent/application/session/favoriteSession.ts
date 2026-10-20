import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { agentRuntime } from "@/features/agent/application/ports/runtimeGateway";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { agentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export function useToggleFavorite(): (
  id: string,
  expectedRevision: number,
  favorite: boolean,
) => Promise<void> {
  return useCallback(async (id, expectedRevision, favorite) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    try {
      await writeAgentSessionSummary(owner, id, expectedRevision, (revision) =>
        runtime.updateSession({ sessionId: id, expectedRevision: revision, favorite }),
      );
    } catch (err) {
      if (!owner.isCurrent()) return;
      void invalidateAgentSessions();
      reportSessionError("favorite", err);
    }
  }, []);
}
