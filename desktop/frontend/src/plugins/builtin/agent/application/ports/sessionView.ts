import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import type { InterruptResumeInput } from "../../domain/hitl";
import type {
  AgentProblem,
  AgentPlan,
  AgentRunView,
  AgentSessionView,
  Message,
} from "@/plugins/sdk/types/agentSessionView";
import type { TranscriptRow } from "../conversation/transcriptRows";
import type { AgentOpeningCommand } from "./runtimeGateway";

export type StopCurrentRootRunAction = () => boolean;
export type SessionProjectionSynchronizationOwnership =
  "after-live" | "replace-live" | "retire-live" | "replace-server";
export type SynchronizeSessionAction = (
  ownership: SessionProjectionSynchronizationOwnership,
) => Promise<boolean>;
export type CancelRunAction = (runId: string) => void;
export type SendAgentInputAction = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
export type ResumeRunAction = (
  runId: string,
  responses: InterruptResumeInput[],
  onSettled?: () => void,
  onStartError?: () => void,
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
  openingCommands: readonly AgentOpeningCommand[];
  retryOpening: ((idempotencyKey: string) => boolean) | null;
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
  useRootNarrativeMessages(): Message[];
  useTranscriptRows(): readonly TranscriptRow[];
  useProblem(): AgentProblem | null;
  useOpeningCommands(): readonly AgentOpeningCommand[];
  usePlan(): AgentProjectionMaterial<AgentPlan>;
  useSharedMaterial<T = unknown>(path?: string): AgentProjectionMaterial<T>;
  useAction(kind: "stop"): StopCurrentRootRunAction | null;
  useAction(kind: "send"): SendAgentInputAction | null;
  getCurrentView(): AgentSessionView;
  getSessions(): Record<string, AgentSessionViewEntry>;
  getSession(sessionId: string): AgentSessionViewEntry | undefined;
  sendToSession(sessionId: string, input: AgentInput, options?: AgentRunStartOptions): boolean;
  dropMessage(sessionId: string, messageId: string): void;
  projectSteerReceipt(
    sessionId: string,
    messageId: string,
    steer: NonNullable<Message["steer"]>,
  ): void;
  reconcileMessageIdentity(
    sessionId: string,
    fromId: string,
    toId: string,
    steerRunId?: string,
  ): void;
  appendLocalUserMessage(sessionId: string, messageId: string, input: AgentInput): void;
  beginViewRefresh(
    sessionId: string,
    invalidateQueuedRunEvents: boolean,
  ): AgentViewRefreshToken | null;
  commitViewRefresh(
    sessionId: string,
    token: AgentViewRefreshToken,
    view: AgentSessionView,
    openingCommands?: readonly AgentOpeningCommand[],
  ): boolean;
  retireProjectionGeneration(sessionIds: readonly string[]): void;
  replaceServerScope(sessionIds: readonly string[]): void;
  clearProblem(sessionId: string): void;
  subscribeSessions(
    onChange: (sessions: Record<string, AgentSessionViewEntry>) => void,
  ): () => void;
}

const port = createSingletonPort<AgentSessionViewPort>("Agent session view port is not configured");

export const configureAgentSessionViewPort = port.configure;
export const agentSessionView = port.get;
