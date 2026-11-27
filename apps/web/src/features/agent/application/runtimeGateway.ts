import { type ApprovalRuleDecision, type RestoreType } from "@/foundation/protocolValues.generated";
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
import { type AgentOpeningAcceptance } from "@/features/agent/application/input/inputSubmission";
export interface AgentSteerInput {
  runId: string;
  userItemId: string;
  input: AgentInput;
}

export interface AgentSessionSnapshot {
  items: AgentItem[];
  runs: AgentRunFact[];
  pendingInterruptSets: AgentPendingInterruptSet[];
  plan?: AgentPlan;
  steerInputs?: AgentSteerInput[];
}

interface SessionRollbackBasis {
  expectedLastRootRunId: string;
  expectedWorkspacePath: string;
}

export interface AgentSessionMaterialRead {
  snapshot: AgentSessionSnapshot;
  rollbackBasis: SessionRollbackBasis;
  projectAssociatedSharedMaterial(shared: Record<string, unknown>): Record<string, unknown>;
  acknowledgeRecoveredCommands?: () => void;
  retiredSteers?: AgentSteerInput[];
  unresolvedSteers?: AgentUnresolvedSteer[];
  unresolvedOpenings?: AgentOpeningCommand[];
}

export interface AgentOpeningRecovery extends AgentOpeningAcceptance {
  releaseObservation?(): Promise<void>;
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
  retry(signal: AbortSignal): Promise<AgentOpeningRecovery>;
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

export type AgentSessionProjectChange = { type: "set"; projectId: string } | { type: "clear" };

export type AgentSessionCreation =
  | { type: "confirmed"; sessionId: string }
  | { type: "unconfirmed"; idempotencyKey: string; retry(): Promise<AgentSessionCreation> };

export interface AgentRuntimeGateway {
  createSession(input: { cwd: string; projectId?: string }): Promise<AgentSessionCreation>;
  sessionExists(sessionId: string, signal: AbortSignal): Promise<boolean>;
  deleteSession(sessionId: string): Promise<void>;
  updateSession(input: {
    sessionId: string;
    expectedRevision: number;
    title?: string;
    favorite?: boolean;
    cwd?: string;
    project?: AgentSessionProjectChange;
  }): Promise<void>;
  forkSession(input: { sessionId: string; fromRunId?: string }): Promise<string>;
  loadSessionSnapshot(
    sessionId: string,
    signal?: AbortSignal,
  ): Promise<AgentSessionMaterialRead | null>;
  loadSessionUsage(sessionId: string, signal?: AbortSignal): Promise<AgentSessionUsage>;
  rollbackSession(
    input: SessionRollbackBasis & {
      sessionId: string;
      toRunId?: string;
      restoreType?: RestoreType;
    },
  ): Promise<{
    droppedRuns: Array<{ runId: string; userInput?: AgentInput }>;
  }>;
  steerRun(runId: string, segmentId: string, input: AgentInput): AgentSteerSubmission;
  isInputReceiptUnknown(error: unknown): boolean;
  isRunGone(error: unknown): boolean;
  isReplayLost(error: unknown): boolean;
  setApprovalMode(mode: ApprovalMode): Promise<ApprovalModeResult>;
  forgetApprovalRule(id: string, expectedDecision: ApprovalRuleDecision): Promise<void>;
}
