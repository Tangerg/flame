import { type AgentRunActivity, type AgentRunView } from "@/features/agent/public/sessionView";
import { type Translate } from "@/lib/i18n/index";
import { toolVerbId } from "@/lib/toolFamilies";
import { terminalSettlementStatus } from "@/features/agent/application/run/rootAttention";
import { isAgentRunFailure } from "@/features/agent/application/view/runOutcome";
export type AgentRunPresentationState = "running" | "waiting" | "finished" | "error" | "canceled";

export function agentRunIsRunning(run: Pick<AgentRunView, "status">): boolean {
  return run.status === "running";
}

export function agentRunIsFinished(run: Pick<AgentRunView, "status">): boolean {
  return run.status === "finished";
}

export function agentRunPresentationState(
  run: Pick<AgentRunView, "status" | "outcome">,
): AgentRunPresentationState {
  return agentRunIsFinished(run) ? terminalSettlementStatus(run.outcome) : run.status;
}

function agentRunActivityLabel(t: Translate, activity: AgentRunActivity): string {
  switch (activity.type) {
    case "callingModel":
    case "modelCallFailed":
    case "applyingUserInput":
      return t(`agent.activity.${activity.type}`);
    case "toolCall": {
      const verb = toolVerbId(activity.tool);
      return verb === undefined ? activity.tool : t(`tool.doing.${verb}`);
    }
  }
}

export function agentRunDetail(t: Translate, run: AgentRunView): string | null {
  if (!agentRunIsFinished(run)) {
    const activity = run.progress?.activity;
    return activity ? agentRunActivityLabel(t, activity) : null;
  }
  if (isAgentRunFailure(run.outcome)) return run.outcome.error.message ?? null;
  switch (run.outcome?.type) {
    case "canceled":
      return run.outcome.detail ?? null;
    case "completed":
    case undefined:
      return null;
  }
}
