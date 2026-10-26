import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { useEffect, useRef } from "react";
import { useAgentSessions } from "@/features/agent/application/session/sessionQueries";

export function useReconcilePersistedAgentSessions(): void {
  const agentAccess = useAgentDependencies();

  const { data, isSuccess } = useAgentSessions();
  const restored = useRef(false);
  useEffect(() => {
    if (!isSuccess) return;
    const sessions = data ?? [];
    if (!restored.current) {
      restored.current = true;
      agentAccess.state.restoreLastSession();
    }
    agentAccess.state.reconcileSessions(sessions.map((session) => session.id));
  }, [isSuccess, data, agentAccess]);
}
