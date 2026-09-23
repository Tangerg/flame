import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentCommandOwner } from "../agentCommandOwner";

export function discardAbandonedDraft(sessionId: string): void {
  const owner = agentCommandOwner();
  const state = agentSessionState();
  const view = agentSessionView();
  const runtime = agentRuntime();
  if (!sessionId || !state.isDraftSession(sessionId)) return;
  if ((view.getSession(sessionId)?.view.messages.length ?? 0) > 0) return;

  void owner
    .settle(runtime.deleteSession(sessionId))
    .then(() => {
      if (owner.isCurrent()) return invalidateAgentSessions();
    })
    .catch((err: unknown) => {
      if (owner.isCurrent()) console.warn("[session] discarding unused draft failed:", err);
    });
}
