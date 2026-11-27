import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { steerRunningTurn } from "./steering";
import { type AgentInputSettlement } from "./inputSubmission";

export function submitAgentInput(
  agentAccess: AgentDependencies,
  sessionId: string,
  input: AgentInput,
  options: AgentRunStartOptions = {},
): Promise<AgentInputSettlement> | null {
  const session = agentAccess.view.getSession(sessionId);
  if (!session?.send || input.parts.length === 0 || session.openingCommands.length) return null;
  const root = selectCurrentRootRun(session.view);
  if (root?.status === "waiting") return null;
  if (root?.status === "running") {
    if (!root.activeSegmentId) return null;
    return steerRunningTurn(agentAccess, {
      sessionId,
      runId: root.id,
      segmentId: root.activeSegmentId,
      input,
    });
  }
  return session.send(input, options);
}

export async function submitCapturedAgentInput(
  agentAccess: AgentDependencies,
  sessionId: string,
  input: AgentInput,
  signal: AbortSignal,
): Promise<AgentInputSettlement> {
  signal.throwIfAborted();
  const submission = submitAgentInput(agentAccess, sessionId, input);
  if (!submission) throw new Error("source session is not ready to accept input");
  return submission;
}
