import type { AgentItem } from "@/plugins/sdk";
import { setTimelineEntry } from "@/plugins/sdk/types/agentTimeline";
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

export function recordToolTimeline(
  state: AgentSessionView,
  item: Extract<AgentItem, { type: "toolCall" }>,
  tool: ToolCall,
): AgentSessionView {
  const entry = itemEntry(item, "tool", item.startedAt);
  if (item.status !== "running" && item.finishedAt !== undefined)
    entry.status = tool.status === "err" ? "err" : tool.status === "denied" ? "declined" : "ok";
  return setTimelineEntry(entry)(state);
}

export function recordCompactionTimeline(
  state: AgentSessionView,
  item: Extract<AgentItem, { type: "compaction" }>,
): AgentSessionView {
  return setTimelineEntry(itemEntry(item, "compaction", item.createdAt))(state);
}
