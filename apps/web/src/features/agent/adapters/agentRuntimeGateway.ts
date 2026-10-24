import { cloneJSON } from "@flame/runtime-client";
import { configureRuntimeOpeningRetries } from "./runtimeOpeningRetries";
import type { RuntimeAgentDriver } from "./runtimeDriver";
import { type MCPServerID } from "@flame/runtime-client/wire";
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
import { configureAgentRuntimeGateway } from "@/features/agent/application/ports/runtimeGateway";
import {
  type AgentRuntimeGateway,
  type AgentOpeningCommand,
  type AgentUnresolvedSteer,
} from "@/features/agent/application/ports/runtimeGateway";
import {
  agentInputToContentBlocks,
  contentBlocksToAgentInput,
} from "@/features/agent/adapters/wireInput";
import { runtimeCapability } from "@/features/connection/public/capabilities";
import { runtimeSessionMaterial } from "@/features/agent/adapters/runtimeSessionMaterial";
import { runtimeItem } from "@/features/agent/adapters/runtimeAgentFacts";
import { toAgentSessionSummary } from "@/features/agent/adapters/runtimeSessionSummary";
import { AgentCommandOwner } from "@/features/agent/application/agentCommandOwner";
import { AgentSessionUsageOwner } from "@/features/agent/application/session/sessionUsage";
class RuntimeAgentGateway implements AgentRuntimeGateway {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #sessionMutations = createMutationSettler();
  // Each Runtime generation owns its opening-retry registry. A snapshot read
  // captures its generation and registers into that registry, so a read that
  // finishes after the gateway switched writes a stale handle into a discarded
  // registry rather than the current one; retries resolve only the current one.
  #generation: {
    retries: Map<string, (signal: AbortSignal) => ReturnType<RuntimeAgentDriver["resume"]>>;
  } = { retries: new Map() };
  retryOpening(key: string, signal: AbortSignal): ReturnType<RuntimeAgentDriver["resume"]> {
    const retry = this.#generation.retries.get(key);
    if (!retry)
      return Promise.reject(
        new Error("opening command does not belong to this Runtime generation"),
      );
    return retry(signal);
  }

  async createSession(input: Parameters<AgentRuntimeGateway["createSession"]>[0]) {
    const client = this.runtimeClient();
    const session = await this.#sessionMutations.settle(
      JSON.stringify(["sessions.create", input.cwd]),
      (signal) => client.sessions.create({ workspace: { path: input.cwd } }, signal),
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
    const includeDescendants = runtimeCapability("subagents");
    try {
      let snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      const commands = await client.runs.recoverCommands(signal);
      if (
        commands.some(
          (command) =>
            command.operation !== "steer" &&
            command.observation.type === "known" &&
            command.observation.acceptance.type !== "notRecorded" &&
            command.observation.acceptance.sessionId === sessionId,
        )
      )
        snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      const material = runtimeSessionMaterial(sessionId, snapshot);
      const unresolvedOpenings: AgentOpeningCommand[] = [];
      const unresolvedSteers: AgentUnresolvedSteer[] = [];
      for (const command of commands) {
        if (command.operation !== "steer") {
          if (
            command.observation.type === "known" &&
            command.observation.acceptance.type !== "notRecorded"
          )
            continue;
          const belongs =
            command.operation === "start"
              ? command.params.sessionId === sessionId
              : snapshot.runs.some((run) => run.id === command.params.runId);
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
                  ...(command.params.params
                    ? { generation: structuredClone({ ...command.params.params }) }
                    : {}),
                }
              : {
                  operation: "resume" as const,
                  runId: command.params.runId,
                  ...(command.params.input
                    ? { input: contentBlocksToAgentInput(command.params.input) }
                    : {}),
                  responses: cloneJSON(command.params.responses),
                };
          if (belongs) {
            generation.retries.set(command.idempotencyKey, async (retrySignal: AbortSignal) => {
              const opening = await command.retry({ signal: retrySignal });
              return {
                ...opening,
                result: {
                  runId: asRunId(opening.result.runId),
                  segmentId: asSegmentId(opening.result.segmentId),
                  ...(opening.result.userItemId
                    ? { userItemId: asItemId(opening.result.userItemId) }
                    : {}),
                },
              };
            });
            unresolvedOpenings.push({
              ...request,
              idempotencyKey: command.idempotencyKey,
              receipt: command.observation.type === "unknown" ? "unknown" : "notRecorded",
            });
          }
          continue;
        }
        try {
          if (command.observation.type === "unknown") throw command.observation.cause;
          const receipt = command.observation.acceptance;
          if (receipt.type !== "notRecorded") {
            if (receipt.sessionId !== sessionId) continue;
            if (receipt.type === "accepted" && receipt.appliedItem) {
              const applied = receipt.appliedItem;
              if (!material.snapshot.items.some((item) => item.id === applied.id))
                material.snapshot.items.push(runtimeItem(applied));
            }
            if (receipt.userItemId && receipt.input) {
              material.snapshot.steerInputs ??= [];
              if (
                !material.snapshot.steerInputs.some(
                  (pending) => pending.userItemId === receipt.userItemId,
                )
              )
                material.snapshot.steerInputs.push({
                  runId: receipt.runId,
                  userItemId: receipt.userItemId,
                  input: contentBlocksToAgentInput(receipt.input),
                  receipt: receipt.type === "retired" ? "retired" : "accepted",
                });
            }
            continue;
          }
        } catch (error) {
          signal?.throwIfAborted();
          console.warn("[session] original steer query failed:", error);
        }
        if (!snapshot.runs.some((run) => run.id === command.params.runId)) continue;
        unresolvedSteers.push({
          idempotencyKey: command.idempotencyKey,
          userItemId: command.userItemId,
          runId: command.params.runId,
          input: contentBlocksToAgentInput(command.params.input),
          retry: () => command.retry(),
        });
      }
      material.unresolvedSteers = unresolvedSteers;
      material.unresolvedOpenings = unresolvedOpenings;
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

  isSteerReceiptUnknown(error: unknown): boolean {
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

  async allowMCPTool(server: MCPServerID, name: string) {
    await this.runtimeClient().approval.setRule({
      tool: { type: "mcp", server, name },
      scope: "global",
      subject: { type: "all" },
      decision: "allow",
    });
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

export function installAgentRuntimeGateway(runtimeClient: () => FlameClient) {
  let commandOwner = AgentCommandOwner.install();
  const gateway = new RuntimeAgentGateway(runtimeClient);
  let usageOwner = AgentSessionUsageOwner.install(gateway);
  const disposePort = configureAgentRuntimeGateway(gateway);
  const disposeRetries = configureRuntimeOpeningRetries({
    retry: (key, signal) => gateway.retryOpening(key, signal),
  });
  let disposed = false;
  return {
    replaceRuntimeGeneration() {
      if (disposed) return;
      gateway.replaceRuntimeGeneration();
      commandOwner = AgentCommandOwner.install();
      usageOwner = AgentSessionUsageOwner.install(gateway);
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      commandOwner.dispose();
      usageOwner.dispose();
      disposePort();
      disposeRetries();
      gateway.dispose();
    },
  };
}
