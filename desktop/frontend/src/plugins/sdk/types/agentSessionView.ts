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

export type ToolCallStatus = "running" | "ok" | "err" | "denied" | "requires-action";
export type AgentSafetyClass = "safe" | "write" | "exec" | "network";

export interface ToolFileChange {
  path: string;
  status: "added" | "deleted" | "modified" | "moved";
  from?: string;
  added: number;
  removed: number;
}

export interface ToolCall {
  id: string;
  runId: string;
  name: string;
  fn: string;
  fnKind?: "path";
  args: string;
  status: ToolCallStatus;
  added?: number;
  removed?: number;
  changes?: ToolFileChange[];
  hits?: number;
  files?: number;
  lines?: number;
  exitCode?: number;
  result?: string;
  command?: string;
  error?: string;
  operation?: string;
  safetyClass?: AgentSafetyClass;
  range?: { start: number; end: number };
  durationMillis?: number;
  approvalDecision?: "approved" | "declined";
}

export interface Message {
  id: string;
  role: MessageRole;
  phase?: AgentMessagePhase;
  createdAt?: string;
  runId: string | null;
  blocks: ContentBlock[];
  steer?: { runId: string; status: "accepted" | "applied" };
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
  usage: RunUsage;
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

export type TimelineEntryKind =
  | "run-start"
  | "run-end"
  | "run-error"
  | "tool"
  | "approval-request"
  | "compaction"
  | "approval-result";

export interface TimelineEntry {
  id: string;
  ts: number;
  kind: TimelineEntryKind;
  runId: string | null;
  summary?: string;
  refId?: string;
  status?: "ok" | "err" | "approved" | "declined";
}

export type PendingInterruptKind = "approval" | "question";

export interface PendingInterrupt {
  itemId: string;
  kind: PendingInterruptKind;
}

export interface PendingInterruptGroup {
  runId: string;
  rootRunId: string;
  sessionId: string;
  interrupts: PendingInterrupt[];
}

export interface AgentSessionView {
  messages: Message[];
  toolCalls: Record<string, ToolCall>;
  runsById: Record<string, AgentRunView>;
  commandError: AgentProblem | null;
  dismissedProblemRunId: string | null;
  assistantTurnByRunId: Record<string, string>;
  timeline: TimelineEntry[];
  pendingInterrupts: PendingInterruptGroup[];
  plan: AgentPlan | null;
  shared: Record<string, unknown>;
}

export const EMPTY_AGENT_SESSION_VIEW: AgentSessionView = {
  messages: [],
  toolCalls: {},
  runsById: {},
  commandError: null,
  dismissedProblemRunId: null,
  assistantTurnByRunId: {},
  timeline: [],
  pendingInterrupts: [],
  plan: null,
  shared: {},
};
