import type { Page, TrajectoryEntry } from "@flame/runtime-contract/wire";

export type TrajectoryViewStatus =
  { type: "loading" } | { type: "ready" } | { type: "failure"; reason: string };

// Settlement joins all reads accepted by the operation, including on failure or cancellation.
export interface TrajectoryViewReads {
  load(signal: AbortSignal): Promise<{ html: string; initial: Page<TrajectoryEntry> }>;
  read(cursor: string | undefined, signal: AbortSignal): Promise<Page<TrajectoryEntry>>;
}
