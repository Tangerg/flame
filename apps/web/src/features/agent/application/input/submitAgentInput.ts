import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import { selectCurrentRootRun } from "@/features/agent/application/view/runTree";
import { recoverUnresolvedSteer, steerRunningTurn } from "./steering";
import {
  acceptedAgentInput,
  recoverOpeningInput,
  type AgentInputSettlement,
} from "./inputSubmission";

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

// Captured components may be recreated after a response loss. Read the existing
// journal projection before accepting another input; it owns the original key.
export async function submitCapturedAgentInput(
  agentAccess: AgentDependencies,
  sessionId: string,
  input: AgentInput,
  signal: AbortSignal,
): Promise<AgentInputSettlement> {
  signal.throwIfAborted();
  if (!agentAccess.view.getSession(sessionId)?.send)
    throw new Error("source session input owner is unavailable");
  const pending = await agentAccess.runtime.loadSessionSnapshot(sessionId, signal);
  signal.throwIfAborted();
  if (!pending) throw new Error("source session is unavailable");
  const inputIdentity = JSON.stringify(input);
  const recovered = pending.recoveredInputs?.find(
    (command) => JSON.stringify(command.input) === inputIdentity,
  );
  if (recovered) return acceptedAgentInput(recovered.acceptance);
  const opening = pending.unresolvedOpenings?.find(
    (command) => command.operation === "start" && JSON.stringify(command.input) === inputIdentity,
  );
  if (opening)
    return {
      status: "unknown",
      idempotencyKey: opening.idempotencyKey,
      recover: () => {
        signal.throwIfAborted();
        return recoverOpeningInput(agentAccess, sessionId, opening.idempotencyKey);
      },
    };
  const steer = pending.unresolvedSteers?.find(
    (command) => JSON.stringify(command.input) === inputIdentity,
  );
  if (steer)
    return {
      status: "unknown",
      idempotencyKey: steer.idempotencyKey,
      recover: () => {
        signal.throwIfAborted();
        return recoverUnresolvedSteer(agentAccess, sessionId, steer);
      },
    };
  if (pending.unresolvedOpenings?.length || pending.unresolvedSteers?.length)
    throw new Error("source session has unresolved input");
  if (pending.recoveredInputs?.length) throw new Error("source session has newly recovered input");
  const submission = submitAgentInput(agentAccess, sessionId, input);
  if (!submission) throw new Error("source session is not ready to accept input");
  return submission;
}
