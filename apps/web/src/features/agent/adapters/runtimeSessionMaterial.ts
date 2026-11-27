import { type RunCommandRecovery, type SessionSnapshot } from "@flame/runtime-client";
import { type SteerInput } from "@flame/runtime-client/wire";
import {
  type AgentOpeningCommand,
  type AgentSessionMaterialRead,
  type AgentSteerInput,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/runtimeGateway";
import { type AgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import { runtimePlan } from "@/features/agent/adapters/runtimePlan";
import {
  runtimeItem,
  runtimePendingInterruptSet,
  runtimeRunFact,
} from "@/features/agent/adapters/runtimeAgentFacts";
import { contentBlocksToAgentInput } from "@/features/agent/adapters/wireInput";
import { disposeAsyncIterable } from "@/lib/asyncOwnership";
export function runtimeSessionMaterial(
  sharedMaterial: AgentSharedMaterial,
  sessionId: string,
  snapshot: SessionSnapshot,
): AgentSessionMaterialRead {
  const plan = snapshot.plan ? runtimePlan(snapshot.plan) : undefined;
  return {
    snapshot: {
      items: snapshot.items.map(runtimeItem),
      runs: snapshot.runs.map(runtimeRunFact),
      pendingInterruptSets: snapshot.interrupts.map(runtimePendingInterruptSet),
      steerInputs: snapshot.steerInputs?.map(runtimeSteerInput),
      ...(plan ? { plan } : {}),
    },
    rollbackBasis: {
      expectedLastRootRunId: snapshot.runs.findLast((run) => !run.parentRunId)?.id ?? "",
      expectedWorkspacePath: snapshot.session.workspace.ref.path,
    },
    projectAssociatedSharedMaterial: sharedMaterial.stage(sessionId, snapshot),
  };
}

type RecoveredSteer = Extract<RunCommandRecovery, { operation: "steer" }>;
type RecoveredOpening = Exclude<RunCommandRecovery, { operation: "steer" }>;

// An unrecorded command names its Session only through its request: a start
// carries it, while resume and steer name a Run of the Session.
export function recoveredCommandBelongs(
  command: RunCommandRecovery,
  sessionId: string,
  runIds: ReadonlySet<string>,
): boolean {
  const { observation } = command;
  if (observation.type === "recorded") return observation.acceptance.sessionId === sessionId;
  if (command.operation === "start")
    return (command.params.sessionId ?? command.params.newSession?.sessionId) === sessionId;
  return runIds.has(command.params.runId);
}

export function runtimeSteerInput({
  runId,
  userItemId,
  input,
}: Pick<SteerInput, "runId" | "userItemId" | "input">): AgentSteerInput {
  return { runId, userItemId, input: contentBlocksToAgentInput(input) };
}

export function unresolvedSteer(command: RecoveredSteer): AgentUnresolvedSteer {
  return {
    idempotencyKey: command.idempotencyKey,
    userItemId: command.userItemId,
    runId: command.params.runId,
    input: contentBlocksToAgentInput(command.params.input),
    retry: () => command.retry(),
  };
}

export function unresolvedOpening(
  command: RecoveredOpening,
  receipt: Exclude<RunCommandRecovery["observation"]["type"], "recorded">,
  sessionId: string,
  assertCurrent: () => void,
): AgentOpeningCommand {
  const request =
    command.operation === "start"
      ? {
          operation: "start" as const,
          input: contentBlocksToAgentInput(command.params.input),
          options: {
            provider: command.params.provider,
            model: command.params.model,
            reasoningEffort: command.params.reasoningEffort,
          },
          ...(command.params.params ? { generation: { ...command.params.params } } : {}),
        }
      : {
          operation: "resume" as const,
          runId: command.params.runId,
          ...(command.params.input
            ? { input: contentBlocksToAgentInput(command.params.input) }
            : {}),
          responses: command.params.responses,
        };
  return {
    ...request,
    idempotencyKey: command.idempotencyKey,
    receipt,
    async retry() {
      assertCurrent();
      const { result, observation } = await command.retry();
      return {
        status: observation.type === "retired" ? "retired" : "accepted",
        sessionId: observation.type === "stream" ? sessionId : observation.sessionId,
        runId: result.runId,
        userItemId: result.userItemId,
        ...(observation.type === "stream"
          ? { releaseObservation: () => disposeAsyncIterable(observation.events) }
          : {}),
      };
    },
  };
}
