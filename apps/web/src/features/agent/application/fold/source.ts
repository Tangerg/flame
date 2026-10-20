import { type AgentEventEnvelope, type AgentItem } from "@/features/agent/public/events";
import { itemStartedAt } from "@/features/agent/application/fold/projections";
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
