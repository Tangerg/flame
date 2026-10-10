import type { AgentMemoryListRequest } from "@flame/runtime-contract/wire";

export type PluginViewStatus =
  { type: "loading" } | { type: "ready" } | { type: "failure"; reason: string };

// Settlement joins all reads accepted by the operation, including on failure or cancellation.
export interface PluginViewReads<T> {
  load(signal: AbortSignal): Promise<{ html: string; initial: T }>;
  read(cursor: string | undefined, signal: AbortSignal): Promise<T>;
}

export type MemoryViewTarget = Pick<AgentMemoryListRequest, "scope" | "workspace">;
