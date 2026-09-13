import type { AgentItem } from "@/plugins/sdk";
import { appendTimelineEntry } from "@/plugins/sdk/types/agentTimeline";
import type {
  AgentSessionView,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";

function itemEntry(item: AgentItem, kind: TimelineEntry["kind"], timestamp: string): TimelineEntry {
  const ts = Date.parse(timestamp);
  if (Number.isNaN(ts))
    throw new Error(`agent.timeline.itemTimestampInvalid:item=${item.id};timestamp=${timestamp}`);
  return { id: `timeline:item:${item.id}:${kind}`, kind, ts, runId: item.runId, refId: item.id };
}

// Transport events and cold snapshots describe the same Item boundaries. Their
// receipt times and event IDs are not the identity or timing of the operation.
export function recordToolTimeline(
  state: AgentSessionView,
  item: Extract<AgentItem, { type: "toolCall" }>,
  tool: ToolCall,
): AgentSessionView {
  const next = appendTimelineEntry(itemEntry(item, "tool-start", item.startedAt))(state);
  if (item.status === "running" || item.finishedAt === undefined) return next;
  return appendTimelineEntry({
    ...itemEntry(item, "tool-end", item.finishedAt),
    status: tool.status === "err" ? "err" : tool.status === "denied" ? "declined" : "ok",
  })(next);
}

export function recordCompactionTimeline(
  state: AgentSessionView,
  item: Extract<AgentItem, { type: "compaction" }>,
): AgentSessionView {
  return appendTimelineEntry(itemEntry(item, "compaction", item.createdAt))(state);
}
