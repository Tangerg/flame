import type { AgentSessionView, TimelineEntry } from "@/plugins/sdk/types/agentSessionView";

type StateUpdate = (state: AgentSessionView) => AgentSessionView;

export const TIMELINE_WINDOW_SIZE = 500;

export function setTimelineEntry(entry: TimelineEntry): StateUpdate {
  return (state) => {
    const index = state.timeline.findIndex((existing) => existing.id === entry.id);
    const next = [...state.timeline];
    if (index === -1) next.push(entry);
    else next[index] = entry;
    next.sort((left, right) => left.ts - right.ts);
    return {
      ...state,
      timeline:
        next.length > TIMELINE_WINDOW_SIZE ? next.slice(next.length - TIMELINE_WINDOW_SIZE) : next,
    };
  };
}
