import type {
  AgentProblem,
  AgentRunMetrics,
  AgentModelSelection,
  AgentRunOutcome,
  AgentRunProgress,
  AgentRunStatus,
  AgentSafetyClass,
  AgentMessagePhase,
  AgentPlan,
} from "./agentSessionView";

export type AgentMessagePart =
  { type: "text"; text: string } | { type: "image"; data: string; mime: string };

export type AgentItemStatus = "running" | "completed" | "incomplete";

export interface AgentQuestionOption {
  label: string;
  description?: string;
  preview?: string;
}

export type AgentQuestionField =
  | { type: "text"; header?: string; prompt: string }
  | {
      type: "choice";
      allowCustom?: boolean;
      header?: string;
      multiple?: boolean;
      options: AgentQuestionOption[];
      prompt: string;
    };

export interface AgentQuestion {
  fields: AgentQuestionField[];
  answers?: string[][];
}

export interface AgentToolInvocation {
  name: string;
  arguments: Record<string, unknown>;
  result?: unknown;
}

export type AgentItem =
  | {
      type: "userMessage";
      content: AgentMessagePart[];
      createdAt: string;
      id: string;
      runId: string;
      status: "completed";
    }
  | {
      type: "agentMessage";
      content?: AgentMessagePart[];
      createdAt: string;
      id: string;
      /** Absent only on the provisional item.started shell. */
      phase?: AgentMessagePhase;
      runId: string;
      status: "running" | "completed";
    }
  | {
      type: "reasoning";
      createdAt: string;
      id: string;
      redacted?: boolean;
      runId: string;
      status: "running" | "completed";
      text?: string;
    }
  | {
      type: "question";
      createdAt: string;
      id: string;
      question: AgentQuestion;
      runId: string;
      status: "completed";
    }
  | {
      type: "toolCall";
      approvalDecision?: "approved" | "declined";
      durationMillis?: number;
      error?: AgentProblem;
      finishedAt?: string;
      id: string;
      runId: string;
      safetyClass?: AgentSafetyClass;
      startedAt: string;
      status: AgentItemStatus;
      tool: AgentToolInvocation;
    }
  | {
      type: "compaction";
      createdAt: string;
      droppedMessages?: number;
      id: string;
      runId: string;
      status: "completed";
      summary: string;
    };

export type AgentItemDelta =
  | { type: "content"; text: string }
  | { type: "reasoning"; text: string }
  | { type: "toolArguments"; argumentsTextDelta: string }
  | { type: "toolOutput"; text: string };

export interface AgentRunFact {
  id: string;
  sessionId: string;
  parentRunId: string | null;
  rootRunId: string;
  spawnedByItemId: string | null;
  status: AgentRunStatus;
  activeSegmentId: string | null;
  outcome: AgentRunOutcome | null;
  /** Exact selection admitted for this Run. Optional at the source boundary so
   * alternate Agent providers can omit a capability they do not expose. */
  modelSelection?: AgentModelSelection;
  metrics: AgentRunMetrics;
  /** Latest authoritative prompt footprint; absent until a model response reports one. */
  contextTokens?: number;
  createdAt: string;
  finishedAt: string | null;
}

export type AgentInterrupt =
  | {
      type: "approval";
      itemId: string;
      runId: string;
      payload: {
        reason?: string;
        rememberable?: boolean;
        tool: AgentToolInvocation;
      };
    }
  | {
      type: "question";
      itemId: string;
      runId: string;
      payload: { question: AgentQuestion };
    };

export interface AgentPendingInterruptSet {
  createdAt: string;
  interrupts: AgentInterrupt[];
  rootRunId: string;
  sessionId: string;
}

export type AgentSegmentOutcome =
  { type: "interrupt"; interrupts: AgentInterrupt[] } | { type: "suspended" } | AgentRunOutcome;

export type AgentStreamEvent =
  | { type: "segment.started"; run: AgentRunFact }
  | { type: "segment.progress"; progress: AgentRunProgress }
  | { type: "segment.finished"; metrics: AgentRunMetrics; outcome: AgentSegmentOutcome }
  | { type: "item.started"; item: AgentItem }
  | { type: "item.delta"; delta: AgentItemDelta; itemId: string }
  | { type: "item.completed"; item: AgentItem }
  | { type: "plan.updated"; plan: AgentPlan };

export interface AgentEventEnvelope {
  event: AgentStreamEvent;
  eventId: string;
  runId: string;
  segmentId: string;
  timestamp: string;
}

export type AgentCancelResult =
  { type: "root"; run: AgentRunFact } | { type: "child"; rootRun: AgentRunFact; run: AgentRunFact };
