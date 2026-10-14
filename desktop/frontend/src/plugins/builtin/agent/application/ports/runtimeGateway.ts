import type { MCPServerID, RestoreType } from "@flame/runtime-contract/wire";
import { createSingletonPort } from "@/lib/ports/singletonPort";
import type {
  AgentItem,
  AgentPendingInterruptSet,
  AgentRunFact,
  AgentDriver,
  AgentRunStartOptions,
} from "@/plugins/sdk";
import type { ApprovalMode, ApprovalModeResult, InterruptResumeInput } from "../../domain/hitl";
import type { AgentInput } from "../../domain/input";
import type { AgentPlan } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionSummary } from "../session/sessionQueries";

export type { RestoreType };

export interface AgentSessionSnapshot {
  items: AgentItem[];
  runs: AgentRunFact[];
  pendingInterruptSets: AgentPendingInterruptSet[];
  plan?: AgentPlan;
  pendingSteers?: Array<{
    runId: string;
    userItemId: string;
    input: AgentInput;
    receipt: "accepted" | "notApplied";
  }>;
}

export interface AgentSessionMaterialRead {
  snapshot: AgentSessionSnapshot;
  projectAssociatedSharedMaterial(shared: Record<string, unknown>): Record<string, unknown>;
  unresolvedSteers?: AgentUnresolvedSteer[];
  unresolvedOpenings?: AgentOpeningCommand[];
}

export type AgentOpeningCommand = (
  | {
      operation: "start";
      input: AgentInput;
      options: AgentRunStartOptions;
      generation?: Record<string, unknown>;
    }
  | { operation: "resume"; runId: string; input?: AgentInput; responses: InterruptResumeInput[] }
) & {
  idempotencyKey: string;
  receipt: "notRecorded" | "unknown";
  retry(signal: AbortSignal): ReturnType<AgentDriver["resume"]>;
};

export interface AgentSessionUsage {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

export interface AgentSteerSubmission extends Promise<{ userItemId: string }> {
  readonly idempotencyKey: string;
  retry(): AgentSteerSubmission;
}

export interface AgentUnresolvedSteer {
  idempotencyKey: string;
  runId: string;
  input: AgentInput;
  retry(): AgentSteerSubmission;
}

export interface AgentRuntimeGateway {
  createSession(input: { cwd: string }): Promise<AgentSessionSummary>;
  deleteSession(sessionId: string): Promise<void>;
  updateSession(input: {
    sessionId: string;
    expectedRevision: number;
    title?: string;
    favorite?: boolean;
    cwd?: string;
  }): Promise<AgentSessionSummary>;
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
  steerRun(runId: string, segmentId: string, input: AgentInput): AgentSteerSubmission;
  isSteerReceiptUnknown(error: unknown): boolean;
  isRunGone(error: unknown): boolean;
  isReplayLost(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<ApprovalModeResult>;
  allowMCPTool(server: MCPServerID, name: string): Promise<void>;
  forgetApprovalRule(id: string): Promise<void>;
}

const port = createSingletonPort<AgentRuntimeGateway>("Agent runtime gateway is not configured");

export const configureAgentRuntimeGateway = port.configure;
export const agentRuntime = port.get;
