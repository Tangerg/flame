import type { Page, TrajectoryEntry } from "@flame/runtime-contract/wire";

export interface TrajectoryViewReads {
  load(signal: AbortSignal): Promise<{ html: string; initial: Page<TrajectoryEntry> }>;
  read(cursor: string | undefined, signal: AbortSignal): Promise<Page<TrajectoryEntry>>;
}
