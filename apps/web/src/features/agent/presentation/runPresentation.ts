import { type AgentRunView } from "@/features/agent/public/sessionView";
import { terminalSettlementStatus } from "@/features/agent/application/run/rootAttention";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
export type AgentRunPresentationState = "running" | "waiting" | "finished" | "error" | "canceled";

export function agentRunPresentationState(
  run: Pick<AgentRunView, "status" | "outcome">,
): AgentRunPresentationState {
  return run.status === "finished" ? terminalSettlementStatus(run.outcome) : run.status;
}

export function agentRunDetail(run: AgentRunView): string | null {
  if (run.status !== "finished") return run.progress?.activity ?? null;
  if (isAgentRunFailure(run.outcome)) return run.outcome.error.message ?? null;
  switch (run.outcome?.type) {
    case "canceled":
      return run.outcome.detail ?? null;
    case "completed":
    case undefined:
      return null;
  }
}
