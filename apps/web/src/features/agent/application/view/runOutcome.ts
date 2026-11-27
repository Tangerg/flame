import {
  type AgentProblem,
  type AgentRunFailureOutcome,
  type AgentRunOutcome,
} from "@/features/agent/public/sessionView";
export function isAgentRunFailure(
  outcome: AgentRunOutcome | null | undefined,
): outcome is AgentRunFailureOutcome {
  return outcome?.type === "timedOut" || outcome?.type === "failed" || outcome?.type === "lost";
}

export function runFailureProblem(outcome: AgentRunFailureOutcome): AgentProblem {
  return outcome.type === "lost" ? { lossReason: outcome.loss.reason } : outcome.error;
}
