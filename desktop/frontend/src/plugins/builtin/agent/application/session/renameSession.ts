import { useCallback } from "react";
import { invalidateAgentSessions, writeAgentSessionSummary } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

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
