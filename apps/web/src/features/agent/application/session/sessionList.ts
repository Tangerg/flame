import { useEffect, useRef } from "react";
import { useAgentSessions } from "@/features/agent/application/session/sessionQueries";
import { agentSessionState } from "@/features/agent/application/ports/sessionState";
export function useReconcilePersistedAgentSessions(): void {
  const { data, isSuccess } = useAgentSessions();
  const restored = useRef(false);
  useEffect(() => {
    if (!isSuccess) return;
    const sessions = data ?? [];
    if (!restored.current) {
      restored.current = true;
      agentSessionState().restoreLastSession();
    }
    agentSessionState().reconcileSessions(sessions.map((session) => session.id));
  }, [isSuccess, data]);
}
