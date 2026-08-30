import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentCommandOwner } from "../agentCommandOwner";

/**
 * An unused draft is invisible in the list AND in the chat while still sitting on the
 * runtime, so without this every visit to the empty-composer screen leaves one behind.
 *
 * Only an UNUSED draft goes: one with messages has already graduated. Fire-and-forget, so a
 * failure stays in the console — the session reappears in the list on the next refetch, no
 * longer draft-marked, and can be deleted like any other.
 */
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
