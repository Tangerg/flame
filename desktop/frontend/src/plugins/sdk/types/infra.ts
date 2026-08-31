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

/** The DURABLE half of `host.notify`: the toast is only the transient visual surface. */
export interface NotificationEntry {
  id: string;
  plugin: string;
  level: NotificationLevel;
  message: string;
  timestamp: number;
  dismissed?: boolean;
}

export type { LogLevel };

/**
 * Query hooks resolve their `queryFn` by looking up the provider for their key, so a plugin
 * can swap the transport without callers knowing. The registry ERASES the result type so
 * every provider fits one map; call sites cast on the way out.
 */
export interface DataProviderSpec<T = unknown, P = unknown> {
  /** Must match the consumer hook's expected key. */
  key: string;
  /** Throw for failure. Query-owned reads receive their generation signal, so a replaced
   *  Runtime cannot retain the cache writer or an underlying transport resource. */
  fetcher: (params?: P, signal?: AbortSignal) => Promise<T>;
}

export interface AgentRunStartOptions {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
}

export interface AgentRunOptionsProviderSpec {
  id: string;
  /** Higher wins. Built-in defaults use 0. */
  priority?: number;
  resolve: () => AgentRunStartOptions;
}

/**
 * The session-bound RPC surface ONLY. Orchestration — pumping events into agentStore,
 * abort and cancel — lives in `useAgentSession`.
 */
export interface AgentDriver {
  /** `userItemId` is the opening userMessage Item's id, used to reconcile the optimistic
   *  bubble by exact identity rather than by content. */
  start: (
    input: ContentBlock[],
    options: AgentRunStartOptions,
    signal?: AbortSignal,
  ) => Promise<
    StreamingResult<{ runId: RunId; segmentId: SegmentId; userItemId: ItemId }, RunEvent>
  >;
  /** Opens a NEW segment of the SAME run — `runId` is unchanged (API.md §6). */
  resume: (
    runId: RunId,
    options: {
      responses: InterruptResponse[];
      /** Committed in the SAME opening as the HITL answers. */
      input?: ContentBlock[];
    },
    signal?: AbortSignal,
  ) => Promise<
    StreamingResult<{ runId: RunId; segmentId: SegmentId; userItemId?: ItemId }, RunEvent>
  >;
}

/** Exactly ONE source is active: the chat resolves the highest `priority` spec, so a user
 *  plugin overrides the built-in by registering above 0. */
export interface AgentSourceSpec {
  id: string;
  label: string;
  /** Higher wins. Built-in defaults use 0. */
  priority?: number;
  /** Builds a FRESH driver per session. */
  factory: () => AgentDriver;
}

// Task types are declared by the store that implements them (`tasksStore`) and
// re-exported here as the plugin-facing contract. Declaring them on this side
// made the edge two-way — the host imports `startTask` from that store — and a
// type is not exempt from direction.
export type { TaskHandle, TaskStartOptions } from "../tasksStore";
