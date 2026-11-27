import type { ApprovalRuleDecision } from "@/foundation/protocolValues.generated";
import { type FlameClient } from "@flame/runtime-client";
import {
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  type MutationSettler,
  isErrorType,
  RpcError,
} from "@flame/runtime-client";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
import {
  type AgentRuntimeGateway,
  type AgentSessionCreation,
  type AgentOpeningCommand,
  type AgentSteerInput,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/runtimeGateway";
import {
  agentInputToContentBlocks,
  contentBlocksToAgentInput,
} from "@/features/agent/adapters/wireInput";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type AgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import {
  recoveredCommandBelongs,
  runtimeSessionMaterial,
  runtimeSteerInput,
  unresolvedOpening,
  unresolvedSteer,
} from "@/features/agent/adapters/runtimeSessionMaterial";
import { type AgentCommands } from "@/features/agent/application/agentCommandOwner";
import { type createAgentSessionUsage } from "@/features/agent/application/session/sessionUsage";

type RuntimeCapabilities = Pick<RuntimeAccess["capabilities"], "hasCapability">;
type CreateSessionMutation = ReturnType<FlameClient["sessions"]["create"]>;

class RuntimeAgentGateway implements AgentRuntimeGateway {
  constructor(
    private readonly runtimeClient: () => FlameClient,
    private readonly capabilities: RuntimeCapabilities,
    private readonly sharedMaterial: AgentSharedMaterial,
    private readonly commands: AgentCommands,
  ) {}

  #sessionMutations = createMutationSettler();
  createSession(input: Parameters<AgentRuntimeGateway["createSession"]>[0]) {
    const client = this.runtimeClient();
    return this.#observeCreation(this.#sessionMutations, (signal) =>
      client.sessions.create(
        {
          workspace: { path: input.cwd },
          ...(input.projectId ? { projectId: input.projectId } : {}),
        },
        signal,
      ),
    );
  }

  async #observeCreation(
    settler: MutationSettler,
    open: (signal: AbortSignal) => CreateSessionMutation,
  ): Promise<AgentSessionCreation> {
    let original: CreateSessionMutation | undefined;
    try {
      const session = await settler.settle((signal) => {
        original = open(signal);
        return original;
      });
      return { type: "confirmed", sessionId: session.id };
    } catch (error) {
      const command = original;
      if (!command || !mutationSettlementIsUnknown(error)) throw error;
      return {
        type: "unconfirmed",
        idempotencyKey: command.idempotencyKey,
        retry: () => this.#observeCreation(settler, (signal) => command.retry({ signal })),
      };
    }
  }

  async deleteSession(sessionId: string) {
    try {
      await this.runtimeClient().sessions.delete(asSessionId(sessionId));
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return;
      throw error;
    }
  }

  async updateSession({
    sessionId,
    cwd,
    ...patch
  }: Parameters<AgentRuntimeGateway["updateSession"]>[0]) {
    await this.runtimeClient().sessions.update({
      sessionId: asSessionId(sessionId),
      ...patch,
      ...(cwd ? { workspace: { path: cwd } } : {}),
    });
  }

  async forkSession(input: Parameters<AgentRuntimeGateway["forkSession"]>[0]) {
    const fork = await this.runtimeClient().sessions.fork({
      sessionId: asSessionId(input.sessionId),
      ...(input.fromRunId ? { fromRunId: asRunId(input.fromRunId) } : {}),
    });
    return fork.id;
  }

  async loadSessionSnapshot(sessionId: string, signal?: AbortSignal) {
    const client = this.runtimeClient();
    const commandOwner = this.commands.current();
    const sid = asSessionId(sessionId);
    const includeDescendants = this.capabilities.hasCapability("childRuns");
    try {
      // Observe receipts before reading material so the transcript and all Run
      // states reflect one Runtime snapshot taken after those observations.
      const commands = await client.runs.recoverCommands(signal);
      signal?.throwIfAborted();
      const snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      const material = runtimeSessionMaterial(this.sharedMaterial, sessionId, snapshot);
      const unresolvedOpenings: AgentOpeningCommand[] = [];
      const retiredSteers: AgentSteerInput[] = [];
      const unresolvedSteers: AgentUnresolvedSteer[] = [];
      const acknowledge: Array<() => void> = [];
      const runIds = new Set(snapshot.runs.map((run) => run.id));
      for (const command of commands) {
        if (!recoveredCommandBelongs(command, sessionId, runIds)) continue;
        const { observation } = command;
        if (observation.type === "recorded") {
          acknowledge.push(observation.acknowledge);
          const receipt = observation.acceptance;
          if (
            command.operation === "steer" &&
            receipt.type === "retired" &&
            receipt.userItemId &&
            receipt.input
          ) {
            retiredSteers.push(
              runtimeSteerInput({
                runId: receipt.runId,
                userItemId: receipt.userItemId,
                input: receipt.input,
              }),
            );
          }
        } else if (command.operation === "steer") {
          if (observation.type === "unknown")
            console.warn("[session] original steer query failed:", observation.cause);
          unresolvedSteers.push(unresolvedSteer(command));
        } else {
          unresolvedOpenings.push(
            unresolvedOpening(command, observation.type, sessionId, () =>
              commandOwner.assertCurrent(),
            ),
          );
        }
      }
      material.retiredSteers = retiredSteers;
      material.unresolvedSteers = unresolvedSteers;
      material.unresolvedOpenings = unresolvedOpenings;
      if (acknowledge.length)
        material.acknowledgeRecoveredCommands = () => {
          for (const command of acknowledge) command();
        };
      return material;
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return null;
      throw error;
    }
  }

  async loadSessionUsage(sessionId: string, signal?: AbortSignal) {
    const report = await this.runtimeClient().usage.session(asSessionId(sessionId), signal);
    return report.total;
  }

  async rollbackSession(input: Parameters<AgentRuntimeGateway["rollbackSession"]>[0]) {
    const response = await this.runtimeClient().sessions.rollback({
      sessionId: asSessionId(input.sessionId),
      expectedLastRootRunId: input.expectedLastRootRunId,
      expectedWorkspacePath: input.expectedWorkspacePath,
      ...(input.toRunId ? { toRunId: asRunId(input.toRunId) } : {}),
      ...(input.restoreType ? { restoreType: input.restoreType } : {}),
    });
    return {
      droppedRuns: response.droppedRuns.map((dropped) => ({
        runId: dropped.run.id,
        ...(dropped.userInput?.length
          ? { userInput: contentBlocksToAgentInput(dropped.userInput) }
          : {}),
      })),
    };
  }

  steerRun(
    runId: string,
    segmentId: string,
    input: Parameters<AgentRuntimeGateway["steerRun"]>[2],
  ) {
    return this.runtimeClient().runs.steer(
      asRunId(runId),
      asSegmentId(segmentId),
      agentInputToContentBlocks(input),
    );
  }

  isInputReceiptUnknown(error: unknown): boolean {
    return !(error instanceof RpcError) || mutationSettlementIsUnknown(error);
  }

  isRunGone(error: unknown) {
    return (
      isErrorType(error, "run_not_found") ||
      isErrorType(error, "run_finished") ||
      isErrorType(error, "run_waiting") ||
      isErrorType(error, "stale_segment")
    );
  }

  isReplayLost(error: unknown) {
    return isErrorType(error, "replay_unavailable") || isErrorType(error, "replay_cursor_invalid");
  }

  async setApprovalMode(mode: Parameters<AgentRuntimeGateway["setApprovalMode"]>[0]) {
    return this.runtimeClient().approval.setMode(mode);
  }

  async forgetApprovalRule(id: string, expectedDecision: ApprovalRuleDecision) {
    await this.runtimeClient().approval.forgetRule({ id, expectedDecision });
  }

  replaceRuntimeGeneration(): void {
    const predecessor = this.#sessionMutations;
    this.#sessionMutations = createMutationSettler();
    predecessor.dispose();
  }

  dispose(): void {
    this.#sessionMutations.dispose();
  }
}

export function createAgentRuntimeGateway(
  runtimeClient: () => FlameClient,
  capabilities: RuntimeCapabilities,
  sharedMaterial: AgentSharedMaterial,
  commands: AgentCommands,
  usage: ReturnType<typeof createAgentSessionUsage>,
) {
  let commandOwner = commands.replace();
  const gateway = new RuntimeAgentGateway(runtimeClient, capabilities, sharedMaterial, commands);
  let usageOwner = usage.replace(gateway);
  let disposed = false;
  return {
    gateway,
    replaceRuntimeGeneration() {
      if (disposed) return;
      gateway.replaceRuntimeGeneration();
      commandOwner = commands.replace();
      usageOwner = usage.replace(gateway);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      commandOwner.dispose();
      usageOwner.dispose();
      gateway.dispose();
    },
  };
}
