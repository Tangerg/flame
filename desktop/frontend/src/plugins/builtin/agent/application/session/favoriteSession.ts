import { useCallback } from "react";
import type { AgentSessionSummary } from "./sessionQueries";
import { queryClient } from "@/lib/queryClient";
import {
  invalidateAgentSessions,
  recoverAgentSessionSummaryField,
  AGENT_SESSIONS_KEY,
} from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandEffect } from "../agentCommandOwner";

export function useToggleFavorite(): (
  id: string,
  expectedRevision: number,
  favorite: boolean,
) => Promise<void> {
  return useCallback(async (id, expectedRevision, favorite) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    let effect: AgentCommandEffect | undefined;
    try {
      await owner.settle(queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] }));
      owner.assertCurrent();
      const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
      queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
        old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
      );
      effect = owner.trackEffect(() =>
        recoverAgentSessionSummaryField(prev, id, "favorite", favorite),
      );
      const updated = await owner.settleSessionSummary(id, expectedRevision, (revision) =>
        runtime.updateSession({
          sessionId: id,
          expectedRevision: revision,
          favorite,
        }),
      );
      owner.assertCurrent();
      queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
        old?.map((s) => (s.id === id ? { ...s, revision: updated.revision } : s)),
      );
      effect.settle();
      void invalidateAgentSessions();
    } catch (err) {
      if (!owner.isCurrent()) return;
      effect?.rollback();
      reportSessionError("favorite", err);
    }
  }, []);
}
