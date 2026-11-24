import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";

// A null Project removes the Session from its Project; Runtime refuses either
// change while a Run could observe it.
export function useSetSessionProject(): (
  id: string,
  expectedRevision: number,
  projectId: string | null,
) => Promise<boolean> {
  const agentAccess = useAgentDependencies();

  return useCallback(
    async (id, expectedRevision, projectId) => {
      const owner = agentAccess.commands.current();
      const runtime = agentAccess.runtime;
      try {
        await writeAgentSessionSummary(owner, id, () =>
          runtime.updateSession({
            sessionId: id,
            expectedRevision,
            project: projectId ? { type: "set", projectId } : { type: "clear" },
          }),
        );
        owner.assertCurrent();
        return true;
      } catch (err) {
        if (!owner.isCurrent()) return false;
        void invalidateAgentSessions();
        reportSessionError(agentAccess.notifications, "project", err);
        return false;
      }
    },
    [agentAccess],
  );
}
