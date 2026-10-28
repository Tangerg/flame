import {
  type RecoveryAction,
  type MessagePhase as AgentMessagePhase,
  type ToolCategory,
  type RunStatus as AgentRunStatus,
  type InterruptType as PendingInterruptKind,
} from "@flame/runtime-client/wire";
export type { AgentMessagePhase, ToolCategory, AgentRunStatus };
import { type CapturedToolResult, type RecordedFileChange } from "@flame/component-sdk";
import { type ContentBlock } from "@/features/agent/public/contentBlock";
export type MessageRole = "user" | "assistant" | "system";

export interface PlanStep {
  readonly id: string;
  readonly text: string;
  readonly status: "done" | "active" | "pending";
}

export interface AgentPlan {
  readonly revision: number;
  readonly steps: readonly PlanStep[];
}

export type ToolCallStatus = CapturedToolResult["status"];

export type AgentSafetyClass = NonNullable<CapturedToolResult["safetyClass"]>;

type Mutable<T> = { -readonly [K in keyof T]: T[K] };

// The reducer advances its current read model; components receive frozen captures.
export type ToolCall = Mutable<Omit<CapturedToolResult, "changes" | "range">> & {
  category?: ToolCategory;
  changes?: Mutable<RecordedFileChange>[];
  range?: Mutable<NonNullable<CapturedToolResult["range"]>>;
};

export interface Message {
  id: string;
  role: MessageRole;
  phase?: AgentMessagePhase;
  createdAt?: string;
  runId: string | null;
  blocks: ContentBlock[];
  /**
   * The addressed Run identifies application; Runtime admission and local
   * delivery observation have separate owners.
   */
  steer?: {
    runId: string;
    receipt?: "accepted" | "retired";
    /** Only a coherent terminal Session read can prove an absent committed Item. */
    application?: "notApplied";
    delivery?: "pending" | "unknown";
  };
}

export interface RunUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  costUsd?: number;
}

export interface AgentProblem {
  message?: string;
  code?: string;
  recovery?: RecoveryAction;
  retryAfterSeconds?: number;
  activeRun?: { runId: string; status: string };
}

export interface AgentUnresolvedEffect {
  processId: string;
  effectId: string;
  cause: string;
  reason?: string;
  detail?: string;
  output?: string;
}

export type AgentRunFailureOutcome = {
  type: "timedOut" | "failed" | "lost";
  error: AgentProblem;
  unresolvedEffects?: AgentUnresolvedEffect[];
};

export type AgentRunOutcome =
  | { type: "completed"; unresolvedEffects?: AgentUnresolvedEffect[] }
  | AgentRunFailureOutcome
  | { type: "canceled"; detail?: string; unresolvedEffects?: AgentUnresolvedEffect[] };

export interface AgentRunMetrics {
  steps: number;
  activeDurationMillis: number;
  /** Absent when the Run reported no model usage; never a zero stand-in. */
  usage?: RunUsage;
}

export interface AgentRunProgress {
  step?: number;
  activity?: string;
  usage?: RunUsage;
  contextTokens?: number;
}

export interface AgentModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface AgentRunView {
  id: string;
  sessionId: string;
  parentRunId: string | null;
  rootRunId: string;
  spawnedByItemId: string | null;
  status: AgentRunStatus;
  activeSegmentId: string | null;
  outcome: AgentRunOutcome | null;
  modelSelection?: AgentModelSelection | null;
  metrics: AgentRunMetrics;
  progress: Omit<AgentRunProgress, "contextTokens"> | null;
  contextTokens: number | null;
  createdAt: string;
  finishedAt: string | null;
}

export interface PendingInterrupt {
  itemId: string;
  kind: PendingInterruptKind;
}

/** The open interrupts one Run raised; the Run itself names its tree's root. */
export interface PendingInterruptGroup {
  runId: string;
  interrupts: PendingInterrupt[];
}

export interface AgentSessionView {
  messages: Message[];
  toolCalls: Record<string, ToolCall>;
  runsById: Record<string, AgentRunView>;
  assistantTurnByRunId: Record<string, string>;
  pendingInterrupts: PendingInterruptGroup[];
  plan: AgentPlan | null;
  shared: Record<string, unknown>;
}

export const EMPTY_AGENT_SESSION_VIEW: AgentSessionView = {
  messages: [],
  toolCalls: {},
  runsById: {},
  assistantTurnByRunId: {},
  pendingInterrupts: [],
  plan: null,
  shared: {},
};
