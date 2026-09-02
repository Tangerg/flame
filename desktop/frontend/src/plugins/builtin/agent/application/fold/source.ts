import type { AgentEventEnvelope, AgentItem } from "@/plugins/sdk";
import type { TimelineEntry } from "@/plugins/sdk/types/agentSessionView";
import { itemStartedAt } from "./projections";

export interface AgentFoldSource {
  runId: string;
  segmentId: string | null;
  eventId: string;
  timestamp: string;
}

export function runEventSource(event: AgentEventEnvelope): AgentFoldSource {
  return {
    runId: event.runId,
    segmentId: event.segmentId,
    eventId: event.eventId,
    timestamp: event.timestamp,
  };
}

export function durableItemSource(item: AgentItem): AgentFoldSource {
  return {
    runId: item.runId,
    segmentId: null,
    eventId: `history:${item.id}:${item.status === "running" ? "started" : "completed"}`,
    timestamp: itemStartedAt(item),
  };
}

export function sourceTimestamp(source: AgentFoldSource): number {
  const timestamp = Date.parse(source.timestamp);
  if (Number.isNaN(timestamp)) {
    throw new Error(
      `agent.fold.timestampInvalid:event=${source.eventId};run=${source.runId};timestamp=${source.timestamp}`,
    );
  }
  return timestamp;
}

/** One timeline entry per (event, kind). Both the run handlers and the item handlers build
 *  these, and they were building them separately from the same four source facts — so the
 *  id scheme that has to stay unique across the whole fold had two authors. */
export function timelineEntry(
  source: AgentFoldSource,
  kind: TimelineEntry["kind"],
  patch: Partial<Omit<TimelineEntry, "id" | "ts" | "kind" | "runId">> = {},
): TimelineEntry {
  return {
    id: `timeline:${source.eventId}:${kind}`,
    ts: sourceTimestamp(source),
    kind,
    runId: source.runId,
    ...patch,
  };
}
