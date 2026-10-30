import { type RestoreType } from "@/foundation/protocolValues.generated";
export type { RestoreType };
import {
  type AgentItem,
  type AgentPendingInterruptSet,
  type AgentRunFact,
} from "@/features/agent/public/events";
import { type AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type ApprovalMode,
  type ApprovalModeResult,
  type InterruptResumeInput,
} from "@/features/agent/domain/hitl";
import { type AgentInput } from "@/features/agent/domain/input";
import { type AgentPlan } from "@/features/agent/public/sessionView";
import { type AgentSessionSummary } from "@/features/agent/application/session/sessionQueries";
import { type AgentOpeningAcceptance } from "@/features/agent/application/input/inputSubmission";
export interface AgentSessionSnapshot {
  items: AgentItem[];
  runs: AgentRunFact[];
  pendingInterruptSets: AgentPendingInterruptSet[];
  plan?: AgentPlan;
  steerInputs?: Array<{
    runId: string;
    userItemId: string;
    input: AgentInput;
    receipt: "accepted" | "retired";
  }>;
}

export interface AgentSessionMaterialRead {
  snapshot: AgentSessionSnapshot;
  projectAssociatedSharedMaterial(shared: Record<string, unknown>): Record<string, unknown>;
  unresolvedSteers?: AgentUnresolvedSteer[];
  unresolvedOpenings?: AgentOpeningCommand[];
  recoveredInputs?: Array<{
    idempotencyKey: string;
    input: AgentInput;
    acceptance: AgentOpeningAcceptance;
  }>;
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
};

export interface AgentSessionUsage {
  inputTokens?: number;
  outputTokens?: number;
  cacheReadTokens?: number;
  cacheWriteTokens?: number;
  reasoningTokens?: number;
  costUsd?: number;
}

export interface AgentSteerSubmission extends Promise<{
  type: "accepted" | "retired";
  userItemId: string;
}> {
  readonly idempotencyKey: string;
  readonly inputIdentity: Promise<string>;
  retry(): AgentSteerSubmission;
}

export interface AgentUnresolvedSteer {
  idempotencyKey: string;
  runId: string;
  userItemId: string | null;
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
  isInputReceiptUnknown(error: unknown): boolean;
  isRunGone(error: unknown): boolean;
  isReplayLost(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<ApprovalModeResult>;
  forgetApprovalRule(id: string): Promise<void>;
}
