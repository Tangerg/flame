import type { LogLevel } from "@/lib/observability/logBridge";
import type {
  ContentBlock,
  InterruptResponse,
  ItemId,
  RunEvent,
  RunId,
  SegmentId,
  StreamingResult,
} from "@/rpc";

export type NotificationLevel = "info" | "warn" | "error";

export interface NotificationEntry {
  id: string;
  plugin: string;
  level: NotificationLevel;
  message: string;
  timestamp: number;
  dismissed?: boolean;
}

export type { LogLevel };

export interface DataProviderSpec<T = unknown, P = unknown> {
  key: string;
  fetcher: (params?: P, signal?: AbortSignal) => Promise<T>;
}

export interface AgentRunStartOptions {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface AgentRunOptionsProviderSpec {
  id: string;
  priority?: number;
  resolve: () => AgentRunStartOptions;
}

export interface AgentDriver {
  start: (
    input: ContentBlock[],
    options: AgentRunStartOptions,
    signal?: AbortSignal,
  ) => Promise<
    StreamingResult<{ runId: RunId; segmentId: SegmentId; userItemId: ItemId }, RunEvent>
  >;
  resume: (
    runId: RunId,
    options: {
      responses: InterruptResponse[];
      input?: ContentBlock[];
    },
    signal?: AbortSignal,
  ) => Promise<
    StreamingResult<{ runId: RunId; segmentId: SegmentId; userItemId?: ItemId }, RunEvent>
  >;
}

export interface AgentSourceSpec {
  id: string;
  label: string;
  priority?: number;
  factory: () => AgentDriver;
}

export type { TaskHandle, TaskStartOptions } from "../tasksStore";
