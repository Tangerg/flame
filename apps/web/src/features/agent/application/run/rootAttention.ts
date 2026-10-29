import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentRunView, type AgentSessionView } from "@/features/agent/public/sessionView";
import { type AgentSessionViewEntry } from "@/features/agent/application/session/sessionAccess";
import {
  selectCurrentRootAttention,
  selectRunProblem,
} from "@/features/agent/application/view/runTree";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
export interface RootRunSettlement {
  sessionId: string;
  runId: string;
  segmentId: string | null;
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
  agentAccess: AgentDependencies,
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

export function projectRootRunSettlement(
  previous: AgentSessionView,
  next: AgentSessionView,
  runId: string,
  segmentId: string | null,
): RootRunSettlement | null {
  if (previous === next) return null;
  const before = previous.runsById[runId];
  const run = next.runsById[runId];
  if (
    !before ||
    !run ||
    run.parentRunId !== null ||
    before.status === run.status ||
    before.status === "finished" ||
    run.status === "running"
  )
    return null;
  return {
    sessionId: run.sessionId,
    runId,
    segmentId,
    status: run.status === "waiting" ? "needsInput" : terminalSettlementStatus(run.outcome),
    errorMessage: run.status === "finished" ? (selectRunProblem(run)?.message ?? null) : null,
  };
}
