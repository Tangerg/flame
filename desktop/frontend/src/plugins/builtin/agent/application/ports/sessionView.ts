import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import type { ApprovalDecision, RememberScope } from "../../domain/hitl";
import type { WireDecision } from "../hitl/wireDecision";
import type {
  AgentProblem,
  AgentPlan,
  AgentRunView,
  AgentSessionView,
  Message,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { AgentRunTreeNode } from "../view/runTree";
import type { TranscriptRow } from "../conversation/transcriptRows";

export type ResolvePatch = {
  decision?: ApprovalDecision;
  answered?: boolean;
  answers?: string[][];
};

export type StopCurrentRootRunAction = () => boolean;
export type SessionProjectionSynchronizationOwnership =
  "after-live" | "replace-live" | "retire-live" | "replace-server";
export type SynchronizeSessionAction = (
  ownership: SessionProjectionSynchronizationOwnership,
) => Promise<boolean>;
export type CancelRunAction = (runId: string) => void;
export type SendAgentInputAction = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
export type InterruptResumePayload =
  | {
      type: "approval";
      decision: WireDecision;
      editedArgs?: Record<string, unknown>;
      remember?: { scope: RememberScope };
    }
  | {
      type: "answer";
      answers: string[][];
    };
export interface InterruptResumeInput {
  itemId: string;
  response: InterruptResumePayload;
}
export type ResumeRunAction = (
  runId: string,
  responses: InterruptResumeInput[],
  onSettled?: () => void,
  onStartError?: () => boolean | void,
) => boolean;

export interface AgentSessionViewEntry {
  view: AgentSessionView;
  viewEpoch: bigint;
  viewRevision: bigint;
  authoritativeRevision: bigint;
  stop: StopCurrentRootRunAction | null;
  send: SendAgentInputAction | null;
  resume: ResumeRunAction | null;
  synchronize: SynchronizeSessionAction | null;
  cancelRun: CancelRunAction | null;
}

export interface AgentViewRefreshToken {
  readonly generation: bigint;
  readonly requestSequence: bigint;
  readonly viewRevision: bigint;
}

export interface AgentProjectionMaterial<T> {
  readonly generation: bigint;
  readonly value: T | undefined;
}

export interface AgentSessionViewPort {
  useCurrentRootRun(): AgentRunView | null;
  useCurrentRootRunning(): boolean;
  useToolCalls(): Record<string, ToolCall>;
  useSessionTimeline(): TimelineEntry[];
  useRootNarrativeMessages(): Message[];
  useTranscriptRows(): readonly TranscriptRow[];
  useRunTree(): AgentRunTreeNode[];
  useProblem(): AgentProblem | null;
  usePlan(): AgentProjectionMaterial<AgentPlan>;
  useSharedMaterial<T = unknown>(path?: string): AgentProjectionMaterial<T>;
  useAction(kind: "stop"): StopCurrentRootRunAction | null;
  useAction(kind: "send"): SendAgentInputAction | null;
  getCurrentView(): AgentSessionView;
  getSessions(): Record<string, AgentSessionViewEntry>;
  getSession(sessionId: string): AgentSessionViewEntry | undefined;
  sendToSession(sessionId: string, input: AgentInput, options?: AgentRunStartOptions): boolean;
  dropMessage(sessionId: string, messageId: string): void;
  reconcileMessageIdentity(sessionId: string, fromId: string, toId: string): void;
  appendLocalUserMessage(sessionId: string, messageId: string, input: AgentInput): void;
  beginViewRefresh(
    sessionId: string,
    invalidateQueuedRunEvents: boolean,
  ): AgentViewRefreshToken | null;
  commitViewRefresh(
    sessionId: string,
    token: AgentViewRefreshToken,
    view: AgentSessionView,
  ): boolean;
  retireProjectionGeneration(sessionIds: readonly string[]): void;
  replaceServerScope(sessionIds: readonly string[]): void;
  clearProblem(sessionId: string): void;
  resolveInterrupt(
    sessionId: string,
    itemId: string,
    settled: ResolvePatch,
    resolvedAt: number,
  ): void;
  subscribeSessions(
    onChange: (sessions: Record<string, AgentSessionViewEntry>) => void,
  ): () => void;
}

const port = createSingletonPort<AgentSessionViewPort>("Agent session view port is not configured");

export const configureAgentSessionViewPort = port.configure;
export const agentSessionView = port.get;
