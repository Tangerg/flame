import type {
  AgentEventEnvelope as RunEvent,
  AgentInterrupt,
  AgentRunFact as RunRef,
  AgentStreamEvent as StreamEvent,
} from "@/plugins/sdk";
import type {
  AgentRunMetrics as RunMetrics,
  AgentRunOutcome,
  AgentSessionView,
} from "@/plugins/sdk/types/agentSessionView";
import { selectCurrentRootRun } from "../view/runTree";
import { reduceAgentEvent } from "./reducer";

export const noMetrics: RunMetrics = {
  steps: 0,
  activeDurationMillis: 0,
};

export const FINISHED_AT = "2026-06-03T00:05:00.000Z";

let nextEventSequence = 0;

function completeStartedRun(state: AgentSessionView, run: RunRef, segmentId: string): RunRef {
  const parent = run.spawnedByItemId ? selectCurrentRootRun(state) : null;
  const { createdAt = "2026-06-03T00:00:00.000Z", metrics = noMetrics, status = "running" } = run;
  const child = run.spawnedByItemId !== null && run.spawnedByItemId !== undefined;
  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: child ? (run.parentRunId ?? parent?.id ?? null) : null,
    rootRunId: child ? (run.rootRunId ?? parent?.rootRunId ?? parent?.id ?? run.id) : run.id,
    spawnedByItemId: run.spawnedByItemId ?? null,
    activeSegmentId: segmentId,
    createdAt,
    metrics,
    status,
    outcome: null,
    finishedAt: null,
  };
}

function completeFinishedRun(state: AgentSessionView, run: RunRef, runId: string): RunRef {
  const owner = state.runsById[runId];
  return {
    ...run,
    id: runId,
    sessionId: owner?.sessionId ?? "ses_1",
    parentRunId: owner?.parentRunId ?? null,
    rootRunId: owner?.rootRunId ?? runId,
    spawnedByItemId: owner?.spawnedByItemId ?? null,
    activeSegmentId: null,
    createdAt: owner?.createdAt ?? "2026-06-03T00:00:00.000Z",
    ...(owner?.modelSelection ? { modelSelection: { ...owner.modelSelection } } : {}),
  };
}

export function testRunEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): RunEvent {
  const payloadRunId =
    event.type === "segment.started"
      ? event.run.id
      : event.type === "item.started" || event.type === "item.completed"
        ? event.item.runId
        : undefined;
  const ownerRunId = runId ?? payloadRunId ?? selectCurrentRootRun(state)?.id ?? "run_1";
  const owner = state.runsById[ownerRunId];
  const ownerSegmentId =
    segmentId ??
    (event.type === "segment.started" ? event.run.activeSegmentId : owner?.activeSegmentId) ??
    `seg_${ownerRunId}`;
  const sequence = ++nextEventSequence;
  const normalizedEvent: StreamEvent =
    event.type === "segment.started"
      ? { ...event, run: completeStartedRun(state, event.run, ownerSegmentId) }
      : event.type === "segment.finished"
        ? { ...event, run: completeFinishedRun(state, event.run, ownerRunId) }
        : event;
  return {
    event: normalizedEvent,
    eventId: `evt_test_${sequence}`,
    runId: ownerRunId,
    segmentId: ownerSegmentId,
    timestamp: `2026-06-03T00:00:${String(sequence % 60).padStart(2, "0")}.000Z`,
  };
}

export function foldTestEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): AgentSessionView {
  return reduceAgentEvent(state, testRunEvent(state, event, runId, segmentId));
}

export const runFinished = (
  outcome: AgentRunOutcome,
  metrics: RunMetrics = noMetrics,
  contextTokens?: number,
): StreamEvent => ({
  type: "segment.finished",
  run: {
    status: "finished",
    outcome,
    metrics,
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    finishedAt: FINISHED_AT,
  } as RunRef,
  interrupts: [],
});

export const runWaiting = (
  interrupts: AgentInterrupt[] = [],
  metrics: RunMetrics = noMetrics,
  contextTokens?: number,
): StreamEvent => ({
  type: "segment.finished",
  run: {
    status: "waiting",
    outcome: null,
    metrics,
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    finishedAt: null,
  } as RunRef,
  interrupts,
});
