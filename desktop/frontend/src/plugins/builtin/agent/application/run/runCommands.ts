import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";

export function useStopCurrentRootRun(): (() => boolean) | null {
  return agentSessionView().useAction("stop");
}

export function stopCurrentRootRun(): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const stop = agentSessionView().getSession(sessionId)?.stop;
  if (!stop) return false;
  return stop();
}

interface RunCommandTarget {
  readonly sessionId: string;
  readonly runId: string;
}

export function cancelSessionRun({ sessionId, runId }: RunCommandTarget): boolean {
  const entry = agentSessionView().getSession(sessionId);
  const run = entry?.view.runsById[runId];
  if (!entry?.cancelRun || !run || run.sessionId !== sessionId || run.status === "finished") {
    return false;
  }
  entry.cancelRun(runId);
  return true;
}

export function dismissActiveSessionProblem(): void {
  const sessionId = agentSessionState().getActiveSessionId();
  agentSessionView().clearProblem(sessionId);
}
