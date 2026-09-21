import type { AgentEventEnvelope, AgentItem } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { measureReduce } from "@/lib/metrics";
import { lookupStreamHandlers, reportPluginError } from "@/plugins/sdk";
import { onItemCompleted, onItemStarted, onItemDelta } from "./itemHandlers";
import { durableItemSource, runEventSource } from "./source";

import { onRunStarted, onRunProgress, onRunFinished } from "./runHandlers";
import { onPlanUpdated } from "./planHandlers";

function applyCoreEvent(state: AgentSessionView, envelope: AgentEventEnvelope): AgentSessionView {
  const event = envelope.event;
  const source = runEventSource(envelope);
  switch (event.type) {
    case "segment.started":
      return onRunStarted(state, event.run, source);
    case "segment.progress":
      return onRunProgress(state, event.progress, source);
    case "segment.finished":
      return onRunFinished(state, event.outcome, event.metrics, event.contextTokens, source);
    case "item.started":
      return onItemStarted(state, event.item, source);
    case "item.delta":
      return onItemDelta(state, event.itemId, event.delta, source);
    case "item.completed":
      return onItemCompleted(state, event.item, source);
    case "plan.updated":
      return onPlanUpdated(state, event.plan);
  }
}

function applyStreamHandlers(state: AgentSessionView, event: AgentEventEnvelope): AgentSessionView {
  const handlers = lookupStreamHandlers(event.event.type);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      next = handler(next, event);
    } catch (error) {
      console.error(`[plugin] stream handler "${event.event.type}" (${pluginName}) threw:`, error);
      reportPluginError(pluginName, "events", error, `event: ${event.event.type}`);
    }
  }
  return next;
}

export function reduceAgentEvent(
  state: AgentSessionView,
  event: AgentEventEnvelope,
): AgentSessionView {
  return measureReduce(event.event.type, () =>
    applyStreamHandlers(applyCoreEvent(state, event), event),
  );
}

export function reduceDurableItem(state: AgentSessionView, item: AgentItem): AgentSessionView {
  const source = durableItemSource(item);
  return item.status === "running"
    ? onItemStarted(state, item, source)
    : onItemCompleted(state, item, source);
}
