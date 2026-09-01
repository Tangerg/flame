// The Runtime owns Session → Run → Item; this keeps one session-scoped narrative while
// preserving each source Run independently.

import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";

export type MessageRole = "user" | "assistant" | "system";
export type AgentMessagePhase = "commentary" | "finalAnswer";

export interface PlanStep {
  readonly id: string;
  readonly text: string;
  readonly status: "done" | "active" | "pending";
}

/** `revision` orders whole REPLACEMENTS; it is never derived from mutable step content. */
export interface AgentPlan {
  readonly revision: number;
  readonly steps: readonly PlanStep[];
}

// `denied` is a user DECISION (HITL decline → `denied_by_user`), not a failure, so it takes
// a neutral treatment rather than the alarming "err" red.
export type ToolCallStatus = "running" | "ok" | "err" | "denied" | "requires-action";
export type AgentSafetyClass = "safe" | "write" | "exec" | "network";

export interface ToolFileChange {
  path: string;
  status: "added" | "deleted" | "modified" | "moved";
  /** Set only for a rename: where the file came from. */
  from?: string;
  added: number;
  removed: number;
}

export interface ToolCall {
  id: string;
  runId: string;
  /** Wire identity, which drives icon/preview routing. The DISPLAY label is `fn`. */
  name: string;
  fn: string;
  /** Set when `fn` is a PATH: the row truncates one from the other end, and only the
   *  projection that filled `fn` knows which case this is. */
  fnKind?: "path";
  /** Accumulated `toolArguments` delta text, pre-parse. */
  args: string;
  status: ToolCallStatus;
  added?: number;
  removed?: number;
  /** What the call's own patch SETS OUT to change, read from its arguments and therefore
   *  known while it runs. An outcome is the receipt's to state, never this. */
  changes?: ToolFileChange[];
  hits?: number;
  files?: number;
  lines?: number;
  /** A non-zero exit does NOT force `status: "err"` — exit≠0 is not always failure (grep
   *  "no match"). Real failures set the Item's `error`. */
  exitCode?: number;
  result?: string;
  /** Separate from `fn`, which carries the human `description`. */
  command?: string;
  error?: string;
  /** Read from the call's ARGUMENTS, not from `args`, which is empty whenever the label
   *  already names the target. */
  operation?: string;
  /** Absent for a tool the runtime has no class for, and read as "not a read" — the same
   *  fail-conservative default the approval gate applies. */
  safetyClass?: AgentSafetyClass;
  /** From the RESULT, not the request: the runtime clamps. Absent for a whole file, where
   *  the span would only restate `lines`. */
  range?: { start: number; end: number };
  step?: string;
  /** Not a formatted ratio: the reader's language decides how "3 of 7" is worded. */
  progress?: { done: number; total: number };
  /** Runtime-measured, excluding approval waits. Absent when unknown: a client stopwatch
   *  would be timing its own render loop. */
  durationMillis?: number;
  /** Absent means the call never crossed a human approval boundary. MUST NOT be inferred
   *  from the current policy or terminal status. */
  approvalDecision?: "approved" | "declined";
}

export interface Message {
  id: string;
  role: MessageRole;
  /** Absent for user/system messages. */
  phase?: AgentMessagePhase;
  /** RAW ISO-8601; formatting belongs to render so a locale change reaches messages already
   *  on screen. Optional because a synthesized turn has no Item, and the fold does NOT reach
   *  for the clock: a client stamp in a runtime-stamped stream skews the date separator. */
  createdAt?: string;
  /** Prevents interleaved child Items from joining a different Run's assistant turn.
   *  `null` on optimistic local bubbles until the real Item reconciles. */
  runId: string | null;
  blocks: ContentBlock[];
}

/** Tokens are INCLUSIVE totals — `inputTokens` already counts the cacheRead portion.
 *  `costUsd` is ABSENT, not 0, when the served model is not in the pricing table, so the
 *  UI shows tokens rather than a fabricated price. */
export interface RunUsage {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  costUsd?: number;
}

export interface AgentProblem {
  /** Absent when the runtime said nothing; the banner then supplies words from `code`,
   *  because a fallback sentence in the fold is one locale's copy in the wrong layer. */
  message?: string;
  /** Everything that branches on the failure reads THIS, never a derived flag. */
  code?: string;
  retryAfterSeconds?: number;
}

export type AgentRunStatus = "running" | "waiting" | "finished";

export type AgentRunFailureOutcome = {
  type: "timedOut" | "failed" | "lost";
  error: AgentProblem;
};

export type AgentRunOutcome =
  | { type: "completed" }
  | AgentRunFailureOutcome
  | { type: "maxSteps"; detail?: string }
  | { type: "maxBudget"; detail?: string }
  | { type: "canceled"; detail?: string };

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

/** FROZEN when the Run is admitted. Session selection can be edited while the Run is still
 *  active, so capability and context-window decisions must prefer this value. */
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
  progress: AgentRunProgress | null;
  createdAt: string;
  finishedAt: string | null;
}

/** Drives the Run Timeline view: the message stream is for READING, the timeline for
 *  AUDITING. Renderers may collapse, filter or group by `runId`. */
export type TimelineEntryKind =
  | "run-start"
  | "run-end"
  | "run-error"
  | "tool-start"
  | "tool-end"
  | "approval-request"
  | "approval-result";

export interface TimelineEntry {
  id: string;
  ts: number;
  kind: TimelineEntryKind;
  runId: string | null;
  summary?: string;
  /** ItemId / reasoningId — used to deeplink and dedupe. */
  refId?: string;
  status?: "ok" | "err" | "approved" | "declined";
}

export type PendingInterruptKind = "approval" | "question";

export interface PendingInterrupt {
  itemId: string;
  kind: PendingInterruptKind;
}

export interface PendingInterruptGroup {
  /** The Run which raised these interrupts. It owns their transcript cards and
   *  tool state, but is not necessarily the Run the resume command addresses. */
  runId: string;
  /** The root which owns the complete pending set. Every group with this value
   *  must be answered together in one resume command. */
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
  /** The open working-narrative message id — reasoning, commentary, and Tool
   *  Items fold together until a user boundary or finalAnswer closes it. Each
   *  Run owns its cursor because root and child Items can arrive interleaved. */
  assistantTurnByRunId: Record<string, string>;
  /** Append-only audit log of run-significant events. See TimelineEntry. */
  timeline: TimelineEntry[];
  /** Pending HITL references for this session. Runtime payloads are
   *  materialized into message blocks at the fold boundary; the read model
   *  retains only the identity and kind needed to resume or settle them. */
  pendingInterrupts: PendingInterruptGroup[];
  /** The Runtime-owned Session Plan, or null before one has been written. */
  plan: AgentPlan | null;
  /** Plugin-owned companion material projected beside the Runtime-owned
   * Session view. Plugins subscribe to generation-bound subtrees through the
   * Agent Session view port. Empty by default. */
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
