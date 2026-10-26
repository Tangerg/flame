import { useAgentDependencies } from "@/features/agent/application/agentContext";
import { type AgentDependencies } from "@/features/agent/application/agentDependencies";

export function useStopCurrentRootRun(): (() => boolean) | null {
  const agentAccess = useAgentDependencies();

  return agentAccess.view.useAction("stop");
}

export function stopCurrentRootRun(agentAccess: AgentDependencies): boolean {
  const sessionId = agentAccess.state.getActiveSessionId();
  const stop = agentAccess.view.getSession(sessionId)?.stop;
  if (!stop) return false;
  return stop();
}

interface RunCommandTarget {
  readonly sessionId: string;
  readonly runId: string;
}

export function cancelSessionRun(
  agentAccess: AgentDependencies,
  { sessionId, runId }: RunCommandTarget,
): boolean {
  const entry = agentAccess.view.getSession(sessionId);
  const run = entry?.view.runsById[runId];
  if (!entry?.cancelRun || !run || run.sessionId !== sessionId || run.status === "finished") {
    return false;
  }
  entry.cancelRun(runId);
  return true;
}

export function dismissActiveSessionProblem(agentAccess: AgentDependencies): void {
  const sessionId = agentAccess.state.getActiveSessionId();
  agentAccess.view.clearProblem(sessionId);
}
