import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { agentRuntime } from "@/features/agent/application/ports/runtimeGateway";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { agentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export function useRenameSession(): (
  id: string,
  expectedRevision: number,
  title: string,
) => Promise<void> {
  return useCallback(async (id, expectedRevision, title) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    try {
      await writeAgentSessionSummary(owner, id, expectedRevision, (revision) =>
        runtime.updateSession({ sessionId: id, expectedRevision: revision, title }),
      );
    } catch (err) {
      if (!owner.isCurrent()) return;
      void invalidateAgentSessions();
      reportSessionError("rename", err);
    }
  }, []);
}
