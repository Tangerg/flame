import type { AgentEventEnvelope, AgentItem } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { measureReduce } from "@/lib/metrics";
import { onItemCompleted, onItemStarted, onItemDelta } from "./itemHandlers";
import { durableItemSource, runEventSource } from "./source";

import { onRunStarted, onRunProgress, onRunFinished } from "./runHandlers";
import { onPlanUpdated } from "./planHandlers";

export function reduceAgentEvent(
  state: AgentSessionView,
  envelope: AgentEventEnvelope,
): AgentSessionView {
  const event = envelope.event;
  return measureReduce(event.type, () => {
    const source = runEventSource(envelope);
    switch (event.type) {
      case "segment.started":
        return onRunStarted(state, event.run, source);
      case "segment.progress":
        return onRunProgress(state, event.progress, source);
      case "segment.finished":
        return onRunFinished(state, event.run, event.interrupts, source);
      case "item.started":
        return onItemStarted(state, event.item, source);
      case "item.delta":
        return onItemDelta(state, event.itemId, event.delta, source);
      case "item.completed":
        return onItemCompleted(state, event.item, source);
      case "plan.updated":
        return onPlanUpdated(state, event.plan);
    }
  });
}

export function reduceDurableItem(state: AgentSessionView, item: AgentItem): AgentSessionView {
  const source = durableItemSource(item);
  return item.status === "running"
    ? onItemStarted(state, item, source)
    : onItemCompleted(state, item, source);
}
