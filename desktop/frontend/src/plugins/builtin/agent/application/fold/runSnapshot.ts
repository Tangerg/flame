import type { AgentRunFact } from "@/plugins/sdk";
import type {
  AgentRunOutcome,
  AgentSessionView,
  TimelineEntry,
} from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry } from "@/plugins/sdk/types/agentTimeline";
import { projectRunRef } from "../view/runProjection";
import { isAgentRunFailure } from "../view/runOutcome";

export function foldRunSnapshot(state: AgentSessionView, run: AgentRunFact): AgentSessionView {
  const projected = projectRunRef(run);
  const previous = state.runsById[run.id];
  const progress =
    previous?.status === "running" &&
    projected.status === "running" &&
    previous.activeSegmentId === projected.activeSegmentId
      ? previous.progress
      : projected.progress;
  // A snapshot states the footprint it knows; a live stream may already have carried a newer
  // one, and the reading must not fall back to nothing when the snapshot omits it.
  const contextTokens = projected.contextTokens ?? previous?.contextTokens ?? null;
  let next: AgentSessionView = {
    ...state,
    runsById: {
      ...state.runsById,
      [run.id]: { ...projected, progress, contextTokens },
    },
  };
  if (!next.timeline.some((entry) => entry.runId === run.id && entry.kind === "run-start")) {
    next = appendTimelineEntry({
      id: `snapshot:run:${run.id}:start`,
      ts: snapshotTimestamp(run.id, projected.createdAt),
      kind: "run-start",
      runId: run.id,
    })(next);
  }
  if (
    projected.status === "finished" &&
    !next.timeline.some(
      (entry) => entry.runId === run.id && (entry.kind === "run-end" || entry.kind === "run-error"),
    )
  ) {
    next = appendTimelineEntry({
      id: `snapshot:run:${run.id}:terminal`,
      ts: snapshotTimestamp(run.id, projected.finishedAt!),
      kind: isAgentRunFailure(projected.outcome) ? "run-error" : "run-end",
      runId: run.id,
      ...terminalTimelinePatch(projected.outcome),
    })(next);
  }
  return next;
}

function terminalTimelinePatch(
  outcome: AgentRunOutcome | null,
): Partial<Pick<TimelineEntry, "status" | "summary">> {
  if (!outcome) return {};
  if (outcome.type === "completed") return { status: "ok" };
  if (isAgentRunFailure(outcome)) {
    return {
      status: "err",
      summary: outcome.error.message ?? outcome.error.code,
    };
  }
  return { summary: outcome.detail ?? outcome.type };
}

function snapshotTimestamp(runId: string, value: string): number {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    throw new Error(`agent.snapshot.runTimestampInvalid:run=${runId};timestamp=${value}`);
  }
  return timestamp;
}
