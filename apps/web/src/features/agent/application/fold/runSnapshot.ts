import { type AgentRunFact } from "@/features/agent/public/events";
import { type AgentSessionView } from "@/features/agent/public/sessionView";
import { projectRunRef } from "@/features/agent/application/view/runProjection";
import { dropRunPendingInterrupts } from "@/features/agent/application/fold/fold";
export function foldRunSnapshot(state: AgentSessionView, run: AgentRunFact): AgentSessionView {
  const projected = projectRunRef(run);
  const previous = state.runsById[run.id];
  const progress =
    previous?.status === "running" &&
    projected.status === "running" &&
    previous.activeSegmentId === projected.activeSegmentId
      ? previous.progress
      : projected.progress;
  const contextTokens = projected.contextTokens ?? previous?.contextTokens ?? null;
  const settled = projected.status === "waiting" ? state : dropRunPendingInterrupts(state, run.id);
  return {
    ...settled,
    runsById: {
      ...state.runsById,
      [run.id]: { ...projected, progress, contextTokens },
    },
  };
}
