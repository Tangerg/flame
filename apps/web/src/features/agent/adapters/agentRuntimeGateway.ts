import type { RuntimeAgentDriver } from "./runtimeDriver";
import { type FlameClient } from "@flame/runtime-client";
import {
  asRunId,
  asItemId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  isErrorType,
  RpcError,
} from "@flame/runtime-client";
import { mutationSettlementIsUnknown } from "@flame/runtime-client";
import {
  type AgentRuntimeGateway,
  type AgentOpeningCommand,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/runtimeGateway";
import {
  agentInputToContentBlocks,
  contentBlocksToAgentInput,
} from "@/features/agent/adapters/wireInput";
import { type RuntimeAccess } from "@/features/connection/public/runtime";
import { type AgentSharedMaterial } from "@/features/agent/application/session/sharedMaterial";
import {
  mergeRecordedCommand,
  recoveredCommandBelongs,
  runtimeSessionMaterial,
  unresolvedOpening,
  unresolvedSteer,
  type RecoveredOpening,
} from "@/features/agent/adapters/runtimeSessionMaterial";
import { toAgentSessionSummary } from "@/features/agent/adapters/runtimeSessionSummary";
import { type AgentCommands } from "@/features/agent/application/agentCommandOwner";
import { type createAgentSessionUsage } from "@/features/agent/application/session/sessionUsage";

type RuntimeCapabilities = Pick<RuntimeAccess["capabilities"], "hasCapability">;

function openingRetry(sessionId: string, command: RecoveredOpening) {
  return {
    sessionId,
    retry: async (signal: AbortSignal) => {
      const opening = await command.retry({ signal });
      return {
        ...opening,
        result: {
          runId: asRunId(opening.result.runId),
          segmentId: asSegmentId(opening.result.segmentId),
          ...(opening.result.userItemId ? { userItemId: asItemId(opening.result.userItemId) } : {}),
        },
      };
    },
  };
}

class RuntimeAgentGateway implements AgentRuntimeGateway {
  constructor(
    private readonly runtimeClient: () => FlameClient,
    private readonly capabilities: RuntimeCapabilities,
    private readonly sharedMaterial: AgentSharedMaterial,
  ) {}

  #sessionMutations = createMutationSettler();
  // Each Runtime generation owns its opening-retry registry. A snapshot read
  // captures its generation and registers into that registry, so a read that
  // finishes after the gateway switched writes a stale handle into a discarded
  // registry rather than the current one; retries resolve only the current one.
  #generation: {
    retries: Map<
      string,
      {
        sessionId: string;
        retry: (signal: AbortSignal) => ReturnType<RuntimeAgentDriver["resume"]>;
      }
    >;
  } = { retries: new Map() };
  retryOpening(
    sessionId: string,
    key: string,
    signal: AbortSignal,
  ): ReturnType<RuntimeAgentDriver["resume"]> {
    const original = this.#generation.retries.get(key);
    if (!original || original.sessionId !== sessionId)
      return Promise.reject(
        new Error("opening command does not belong to this Runtime generation"),
      );
    return original.retry(signal);
  }

  async createSession(input: Parameters<AgentRuntimeGateway["createSession"]>[0]) {
    const client = this.runtimeClient();
    const session = await this.#sessionMutations.settle(
      JSON.stringify(["sessions.create", input.cwd, input.projectId ?? null]),
      (signal) =>
        client.sessions.create(
          {
            workspace: { path: input.cwd },
            ...(input.projectId ? { projectId: input.projectId } : {}),
          },
          signal,
        ),
    );
    return toAgentSessionSummary(session);
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
    const updated = await this.runtimeClient().sessions.update({
      sessionId: asSessionId(sessionId),
      ...patch,
      ...(cwd ? { workspace: { path: cwd } } : {}),
    });
    return toAgentSessionSummary(updated);
  }

  async forkSession(input: Parameters<AgentRuntimeGateway["forkSession"]>[0]) {
    const fork = await this.runtimeClient().sessions.fork({
      sessionId: asSessionId(input.sessionId),
      ...(input.fromRunId ? { fromRunId: asRunId(input.fromRunId) } : {}),
    });
    return { id: fork.id };
  }

  async loadSessionSnapshot(sessionId: string, signal?: AbortSignal) {
    const client = this.runtimeClient();
    const generation = this.#generation;
    const sid = asSessionId(sessionId);
    const includeDescendants = this.capabilities.hasCapability("childRuns");
    try {
      let snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      const commands = await client.runs.recoverCommands(signal);
      signal?.throwIfAborted();
      if (
        commands.some(
          (command) =>
            command.operation !== "steer" &&
            command.observation.type === "recorded" &&
            command.observation.acceptance.sessionId === sessionId,
        )
      )
        snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      const material = runtimeSessionMaterial(this.sharedMaterial, sessionId, snapshot);
      const unresolvedOpenings: AgentOpeningCommand[] = [];
      const unresolvedSteers: AgentUnresolvedSteer[] = [];
      const acknowledge: Array<() => void> = [];
      const runIds = new Set(snapshot.runs.map((run) => run.id));
      for (const command of commands) {
        if (!recoveredCommandBelongs(command, sessionId, runIds)) continue;
        if (command.operation !== "steer")
          generation.retries.set(command.idempotencyKey, openingRetry(sessionId, command));
        const { observation } = command;
        if (observation.type === "recorded") {
          acknowledge.push(observation.acknowledge);
          mergeRecordedCommand(material, command, observation);
        } else if (command.operation === "steer") {
          if (observation.type === "unknown")
            console.warn("[session] original steer query failed:", observation.cause);
          unresolvedSteers.push(unresolvedSteer(command));
        } else unresolvedOpenings.push(unresolvedOpening(command, observation.type));
      }
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

  async forgetApprovalRule(id: string) {
    await this.runtimeClient().approval.forgetRule(id);
  }

  replaceRuntimeGeneration(): void {
    this.#generation = { retries: new Map() };
    const predecessor = this.#sessionMutations;
    this.#sessionMutations = createMutationSettler();
    predecessor.dispose();
  }

  dispose(): void {
    this.#generation = { retries: new Map() };
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
  const gateway = new RuntimeAgentGateway(runtimeClient, capabilities, sharedMaterial);
  let usageOwner = usage.replace(gateway);
  let disposed = false;
  return {
    gateway,
    retries: {
      retry: (sessionId: string, key: string, signal: AbortSignal) =>
        gateway.retryOpening(sessionId, key, signal),
    },
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
