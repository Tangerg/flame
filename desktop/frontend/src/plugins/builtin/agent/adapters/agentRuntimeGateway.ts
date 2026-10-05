import type { MCPServerID } from "@flame/runtime-contract/wire";
import type { FlameClient } from "@flame/runtime-contract/client";
import {
  asRunId,
  asSegmentId,
  asSessionId,
  createMutationSettler,
  isErrorType,
} from "@flame/runtime-contract/client";
import { configureAgentRuntimeGateway } from "../application/ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../application/ports/runtimeGateway";
import { agentInputToContentBlocks, contentBlocksToAgentInput } from "./wireInput";
import { runtimeCapability } from "@/plugins/builtin/runtime/public/capabilities";
import { runtimeSessionMaterial } from "./runtimeSessionMaterial";
import { AgentCommandOwner } from "../application/agentCommandOwner";
import { AgentSessionUsageOwner } from "../application/session/sessionUsage";

class RuntimeAgentGateway implements AgentRuntimeGateway {
  constructor(private readonly runtimeClient: () => FlameClient) {}

  #sessionMutations = createMutationSettler();

  async createSession(input: Parameters<AgentRuntimeGateway["createSession"]>[0]) {
    const client = this.runtimeClient();
    const session = await this.#sessionMutations.settle(
      JSON.stringify(["sessions.create", input.cwd]),
      (signal) => client.sessions.create({ workspace: { path: input.cwd } }, signal),
    );
    return { id: session.id };
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
    return { revision: updated.revision };
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
    const sid = asSessionId(sessionId);
    const includeDescendants = runtimeCapability("subagents");
    try {
      const snapshot = await client.sessions.snapshot(sid, includeDescendants, signal);
      return runtimeSessionMaterial(sessionId, snapshot);
    } catch (error) {
      if (isErrorType(error, "session_not_found")) return null;
      throw error;
    }
  }

  loadSessionUsage(sessionId: string, signal?: AbortSignal) {
    return this.runtimeClient().usage.session(asSessionId(sessionId), signal);
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

  async steerRun(
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
    return (await this.runtimeClient().approval.setMode(mode)).mode;
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
    const predecessor = this.#sessionMutations;
    this.#sessionMutations = createMutationSettler();
    predecessor.dispose();
  }

  dispose(): void {
    this.#sessionMutations.dispose();
  }
}

export function installAgentRuntimeGateway(runtimeClient: () => FlameClient) {
  let commandOwner = AgentCommandOwner.install();
  const gateway = new RuntimeAgentGateway(runtimeClient);
  let usageOwner = AgentSessionUsageOwner.install(gateway);
  const disposePort = configureAgentRuntimeGateway(gateway);
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
      gateway.dispose();
    },
  };
}
