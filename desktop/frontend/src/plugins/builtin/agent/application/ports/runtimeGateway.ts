import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentItem, AgentPendingInterruptSet, AgentRunFact } from "@/plugins/sdk";
import type { ApprovalMode } from "../../domain/hitl";
import type { AgentInput } from "../../domain/input";
import type { AgentPlan } from "@/plugins/sdk/types/agentSessionView";

export type RestoreType = "history" | "files" | "both";

export interface AgentSessionSnapshot {
  items: AgentItem[];
  runs: AgentRunFact[];
  pendingInterruptSets: AgentPendingInterruptSet[];
  plan?: AgentPlan;
}

export interface AgentSessionMaterialRead {
  snapshot: AgentSessionSnapshot;
  projectAssociatedSharedMaterial(shared: Record<string, unknown>): Record<string, unknown>;
}

export interface AgentSessionUsage {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

export interface AgentRuntimeGateway {
  createSession(input: { cwd: string }): Promise<{ id: string }>;
  deleteSession(sessionId: string): Promise<void>;
  updateSession(input: {
    sessionId: string;
    expectedRevision: number;
    title?: string;
    favorite?: boolean;
    cwd?: string;
  }): Promise<{ revision: number }>;
  forkSession(input: { sessionId: string; fromRunId?: string }): Promise<{ id: string }>;
  loadSessionSnapshot(
    sessionId: string,
    signal?: AbortSignal,
  ): Promise<AgentSessionMaterialRead | null>;
  loadSessionUsage(sessionId: string, signal?: AbortSignal): Promise<AgentSessionUsage>;
  rollbackSession(input: {
    sessionId: string;
    toRunId?: string;
    restoreType?: RestoreType;
  }): Promise<{
    droppedRuns: Array<{ runId: string; userInput?: AgentInput }>;
  }>;
  steerRun(runId: string, segmentId: string, input: AgentInput): Promise<{ userItemId: string }>;
  isRunGone(error: unknown): boolean;
  isReplayLost(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<ApprovalMode>;
  forgetApprovalRule(id: string): Promise<void>;
}

const port = createSingletonPort<AgentRuntimeGateway>("Agent runtime gateway is not configured");

export const configureAgentRuntimeGateway = port.configure;
export const agentRuntime = port.get;
