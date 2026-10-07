import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { terminalSettlementStatus } from "../application/run/rootAttention";
import { isAgentRunFailure } from "../application/view/runOutcome";

export type AgentRunPresentationState = "running" | "waiting" | "finished" | "error" | "canceled";

export function agentRunPresentationState(run: AgentRunView): AgentRunPresentationState {
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
