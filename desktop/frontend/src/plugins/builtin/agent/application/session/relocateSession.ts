import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { rpcErrorText } from "@/lib/rpcErrors";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

export function useRelocateSession(): (
  id: string,
  expectedRevision: number,
  cwd: string,
) => Promise<boolean> {
  return useCallback(async (id, expectedRevision, cwd) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    try {
      await owner.settleSessionSummary(id, expectedRevision, (revision) =>
        runtime.updateSession({ sessionId: id, expectedRevision: revision, cwd }),
      );
      owner.assertCurrent();
      await invalidateAgentSessions();
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
