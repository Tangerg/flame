import { useCallback } from "react";
import {
  invalidateAgentSessions,
  writeAgentSessionSummary,
} from "@/features/agent/application/session/sessionQueries";
import { rpcErrorText } from "@/lib/rpcErrors";
import { agentRuntime } from "@/features/agent/application/ports/runtimeGateway";
import { reportSessionError } from "@/features/agent/application/session/reportSessionError";
import { agentCommandOwner } from "@/features/agent/application/agentCommandOwner";
export function useRelocateSession(): (
  id: string,
  expectedRevision: number,
  cwd: string,
) => Promise<boolean> {
  return useCallback(async (id, expectedRevision, cwd) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
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
  }, []);
}
