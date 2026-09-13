import type { AgentSessionView, TimelineEntry } from "@/plugins/sdk/types/agentSessionView";

type StateUpdate = (state: AgentSessionView) => AgentSessionView;

export const TIMELINE_WINDOW_SIZE = 500;

/** Insert one idempotent entry in server-time order and retain the newest
 * bounded window. Stable sorting preserves source order for equal timestamps. */
export function appendTimelineEntry(entry: TimelineEntry): StateUpdate {
  return (state) => {
    if (state.timeline.some((existing) => existing.id === entry.id)) return state;
    const next = [...state.timeline, entry].sort((left, right) => left.ts - right.ts);
    return {
      ...state,
      timeline:
        next.length > TIMELINE_WINDOW_SIZE ? next.slice(next.length - TIMELINE_WINDOW_SIZE) : next,
    };
  };
}
