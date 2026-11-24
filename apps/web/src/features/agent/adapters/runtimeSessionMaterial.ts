import { type RunCommandRecovery, type SessionSnapshot } from "@flame/runtime-client";
import {
  type AgentOpeningCommand,
  type AgentSessionMaterialRead,
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
      steerInputs: snapshot.steerInputs?.map((steer) => ({
        runId: steer.runId,
        userItemId: steer.userItemId,
        input: contentBlocksToAgentInput(steer.input),
        receipt: "accepted",
      })),
      ...(plan ? { plan } : {}),
    },
    projectAssociatedSharedMaterial: sharedMaterial.stage(sessionId, snapshot),
  };
}

type RecordedObservation = Extract<RunCommandRecovery["observation"], { type: "recorded" }>;
type RecoveredSteer = Extract<RunCommandRecovery, { operation: "steer" }>;
export type RecoveredOpening = Exclude<RunCommandRecovery, { operation: "steer" }>;

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

// A recorded command is merged into the snapshot it may predate: its input is
// kept for the composer, and an accepted steer contributes its applied item and
// pending input once.
export function mergeRecordedCommand(
  material: AgentSessionMaterialRead,
  command: RunCommandRecovery,
  observation: RecordedObservation,
): void {
  const receipt = observation.acceptance;
  if (command.operation !== "resume") {
    material.recoveredInputs ??= [];
    material.recoveredInputs.push({
      idempotencyKey: command.idempotencyKey,
      input: contentBlocksToAgentInput(command.params.input),
      acceptance: {
        status: receipt.type,
        sessionId: receipt.sessionId,
        runId: receipt.runId,
        ...(receipt.userItemId ? { userItemId: receipt.userItemId } : {}),
      },
      acknowledge: observation.acknowledge,
    });
  }
  if (command.operation !== "steer") return;
  if (receipt.type === "accepted" && receipt.appliedItem) {
    const applied = receipt.appliedItem;
    if (!material.snapshot.items.some((item) => item.id === applied.id))
      material.snapshot.items.push(runtimeItem(applied));
  }
  if (!receipt.userItemId || !receipt.input) return;
  material.snapshot.steerInputs ??= [];
  if (!material.snapshot.steerInputs.some((pending) => pending.userItemId === receipt.userItemId))
    material.snapshot.steerInputs.push({
      runId: receipt.runId,
      userItemId: receipt.userItemId,
      input: contentBlocksToAgentInput(receipt.input),
      receipt: receipt.type,
    });
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
  return { ...request, idempotencyKey: command.idempotencyKey, receipt };
}
