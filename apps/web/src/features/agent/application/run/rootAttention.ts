import { type AgentAccess } from "@/features/agent/public/agent";
import { type AgentRunView, type AgentSessionView } from "@/features/agent/public/sessionView";
import { type AgentSessionViewEntry } from "@/features/agent/application/session/sessionAccess";
import { type AgentRootAttention } from "@/features/agent/application/view/runTree";
import {
  selectCurrentRootAttention,
  selectCurrentRootRun,
  selectRunProblem,
} from "@/features/agent/application/view/runTree";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
export interface RootRunSettlement {
  sessionId: string;
  status: "needsInput" | "finished" | "error" | "canceled";
  errorMessage: string | null;
}

function currentRootRunning(view: AgentSessionView): boolean {
  return selectCurrentRootAttention(view).status === "running";
}

function anySessionRunning(sessions: Record<string, AgentSessionViewEntry>): boolean {
  for (const id in sessions) {
    if (currentRootRunning(sessions[id]!.view)) return true;
  }
  return false;
}

export function terminalSettlementStatus(
  outcome: AgentRunView["outcome"],
): Exclude<RootRunSettlement["status"], "needsInput"> {
  if (isAgentRunFailure(outcome)) return "error";
  switch (outcome?.type) {
    case "canceled":
      return "canceled";
    default:
      return "finished";
  }
}

export function subscribeAnySessionRunning(
  agentAccess: AgentAccess,
  onChange: (running: boolean) => void,
): () => void {
  let lastRunning = anySessionRunning(agentAccess.view.getSessions());
  onChange(lastRunning);
  return agentAccess.view.subscribeSessions((sessions) => {
    const running = anySessionRunning(sessions);
    if (running === lastRunning) return;
    lastRunning = running;
    onChange(running);
  });
}

export function subscribeRootRunSettlements(
  agentAccess: AgentAccess,
  onSettled: (settlement: RootRunSettlement) => void,
): () => void {
  const previousBySession = new Map<string, AgentRootAttention>();
  for (const [sessionId, entry] of Object.entries(agentAccess.view.getSessions())) {
    previousBySession.set(sessionId, selectCurrentRootAttention(entry.view));
  }
  return agentAccess.view.subscribeSessions((sessions) => {
    for (const sessionId in sessions) {
      const view = sessions[sessionId]!.view;
      const current = selectCurrentRootAttention(view);
      const previous = previousBySession.get(sessionId) ?? { status: "idle", runId: null };
      if (current.runId === previous.runId && current.status === previous.status) continue;
      previousBySession.set(sessionId, current);

      const sameRunSettled =
        previous.runId !== null &&
        previous.runId === current.runId &&
        (previous.status === "running" || previous.status === "waiting") &&
        (current.status === "waiting" || current.status === "finished");
      if (sameRunSettled) {
        const root = selectCurrentRootRun(view);
        onSettled({
          sessionId,
          status:
            current.status === "waiting"
              ? "needsInput"
              : terminalSettlementStatus(root?.outcome ?? null),
          errorMessage:
            current.status === "finished" ? (selectRunProblem(root)?.message ?? null) : null,
        });
      }
    }
    for (const sessionId of [...previousBySession.keys()]) {
      if (!(sessionId in sessions)) previousBySession.delete(sessionId);
    }
  });
}
