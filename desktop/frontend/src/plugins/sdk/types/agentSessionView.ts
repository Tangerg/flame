import type { CapturedToolResult, RecordedFileChange } from "@flame/plugin-sdk";
import type { RecoveryAction } from "@flame/runtime-contract/wire";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";

export type MessageRole = "user" | "assistant" | "system";
export type AgentMessagePhase = "commentary" | "finalAnswer";

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
  /** The Run this input steered; it is applied once it is that Run's Item. */
  steer?: { runId: string; receipt: "pending" | "unknown" | "accepted" };
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

export type AgentRunStatus = "running" | "waiting" | "finished";

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

type PendingInterruptKind = "approval" | "question";

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
