import { useAgent, type AgentAccess } from "@/features/agent/public/agent";

export function useStopCurrentRootRun(): (() => boolean) | null {
  const agentAccess = useAgent();

  return agentAccess.view.useAction("stop");
}

export function stopCurrentRootRun(agentAccess: AgentAccess): boolean {
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
  agentAccess: AgentAccess,
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

export function dismissActiveSessionProblem(agentAccess: AgentAccess): void {
  const sessionId = agentAccess.state.getActiveSessionId();
  agentAccess.view.clearProblem(sessionId);
}
