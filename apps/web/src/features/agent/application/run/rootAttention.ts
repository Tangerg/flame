import { type LossReason } from "@/foundation/protocolValues.generated";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentRunView, type AgentSessionView } from "@/features/agent/public/sessionView";
import { type AgentSessionViewEntry } from "@/features/agent/application/session/sessionAccess";
import {
  selectCurrentRootAttention,
  selectRunProblem,
} from "@/features/agent/application/view/runTree";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
import type { RootRunSettlement as ComponentRootRunSettlement } from "@flame/component-sdk";
export interface RootRunSettlement extends ComponentRootRunSettlement {
  runId: string;
  segmentId: string | null;
  /** Present when the Run was lost; the client words it. */
  lossReason?: LossReason;
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
  const problem = run.status === "finished" ? selectRunProblem(run) : null;
  return {
    sessionId: run.sessionId,
    runId,
    segmentId,
    status: run.status === "waiting" ? "needsInput" : terminalSettlementStatus(run.outcome),
    errorMessage: problem?.message ?? null,
    ...(problem?.lossReason ? { lossReason: problem.lossReason } : {}),
  };
}
