import { type AgentEventEnvelope, type AgentItem } from "@/features/agent/public/events";
import { itemStartedAt } from "@/features/agent/application/fold/projections";
export interface AgentFoldSource {
  kind: "event" | "durable";
  runId: string;
  segmentId: string | null;
  eventId: string;
  timestamp: string;
}

export function runEventSource(event: AgentEventEnvelope): AgentFoldSource {
  return {
    kind: "event",
    runId: event.runId,
    segmentId: event.segmentId,
    eventId: event.eventId,
    timestamp: event.timestamp,
  };
}

export function durableItemSource(item: AgentItem): AgentFoldSource {
  return {
    kind: "durable",
    runId: item.runId,
    segmentId: null,
    eventId: `history:${item.id}:${item.status === "running" ? "started" : "completed"}`,
    timestamp: itemStartedAt(item),
  };
}
