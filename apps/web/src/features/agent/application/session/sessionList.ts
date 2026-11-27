import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useEffect, useRef } from "react";
import { useAgentSessions } from "@/features/agent/application/session/sessionQueries";

export function useReconcilePersistedAgentSessions(): void {
  const agentAccess = useAgentDependencies();

  const { data, isSuccess } = useAgentSessions();
  const restored = useRef(false);
  useEffect(() => {
    if (!isSuccess || !data) return;
    const { state, runtime, commands } = agentAccess;
    if (!restored.current) {
      restored.current = true;
      state.restoreLastSession();
    }
    const known = new Set(data.map(({ id }) => id));
    const { activeSessionId, openSessionIds } = state.getLifecycleSnapshot();
    const candidates = new Set(openSessionIds);
    if (activeSessionId) candidates.add(activeSessionId);
    const controller = new AbortController();
    const owner = commands.current();
    const isCurrent = () => !controller.signal.aborted && owner.isCurrent();
    // A catalog read can predate the receipt that opened the Session.
    for (const id of candidates) {
      if (known.has(id)) continue;
      void runtime.loadSessionSummary(id, controller.signal).then(
        (session) => {
          if (!session && isCurrent()) state.deleteSession(id);
        },
        (error: unknown) => {
          if (isCurrent()) console.warn("[agent] Could not verify Session:", id, error);
        },
      );
    }
    return () => controller.abort();
  }, [isSuccess, data, agentAccess]);
}
