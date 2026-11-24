import { canonicalJSON } from "@frame/sdk/json";
import { type AgentInterrupt, type AgentRunFact } from "@/features/agent/public/events";
import {
  type AgentRunProgress,
  type AgentRunView,
  type AgentSessionView,
} from "@/features/agent/public/sessionView";
import {
  dropRunPendingInterrupts,
  mergeRunPendingInterrupts,
} from "@/features/agent/application/fold/fold";
import { materializeInterrupt } from "@/features/agent/application/fold/interruptMaterialization";
import { foldRunSnapshot } from "@/features/agent/application/fold/runSnapshot";
import { type AgentFoldSource } from "@/features/agent/application/fold/source";
import {
  projectFinishedRun,
  projectStartedRun,
} from "@/features/agent/application/view/runProjection";

// Live and durable finishes pass through one runtime projection, so a replayed
// finish encodes exactly like the one already folded.
function settledFacts(run: AgentRunView) {
  return canonicalJSON({
    status: run.status,
    finishedAt: run.finishedAt,
    metrics: run.metrics,
    outcome: run.status === "finished" ? run.outcome : null,
  });
}

function isDuplicateRunFinish(
  state: AgentSessionView,
  run: AgentRunView,
  interrupts: readonly AgentInterrupt[],
): boolean {
  const previous = state.runsById[run.id];
  if (!previous || previous.status === "running") return false;
  if (settledFacts(previous) !== settledFacts(run)) return false;
  if (run.status === "finished") return run.outcome !== null;
  const open = new Set(
    state.pendingInterrupts
      .filter((group) => group.runId === run.id)
      .flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId)),
  );
  return interrupts.every((interrupt) => open.has(interrupt.itemId));
}

function statedFootprint(tokens: number | undefined): number | null {
  return tokens !== undefined && tokens > 0 ? tokens : null;
}

function updateRun(
  state: AgentSessionView,
  runId: string,
  update: (run: AgentRunView) => AgentRunView,
): AgentSessionView {
  const run = state.runsById[runId];
  if (!run) {
    throw new Error(`agent.fold.runMissing:event=segment.progress;run=${runId}`);
  }
  return {
    ...state,
    runsById: { ...state.runsById, [runId]: update(run) },
  };
}

export function onRunStarted(
  state: AgentSessionView,
  run: AgentRunFact,
  source: AgentFoldSource,
): AgentSessionView {
  const started = projectStartedRun(run, source);
  const previous = state.runsById[run.id];
  if (previous) {
    if (previous.status === "finished") {
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.started;run=${run.id};status=finished;expected=waitingOrAbsent`,
      );
    }
    if (previous.status === "running") {
      if (previous.activeSegmentId === source.segmentId) return state;
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.started;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${previous.activeSegmentId ?? "missing"}`,
      );
    }
  }
  return {
    ...dropRunPendingInterrupts(state, run.id),
    runsById: {
      ...state.runsById,
      [run.id]: started,
    },
  };
}

export function onRunProgress(
  state: AgentSessionView,
  progress: AgentRunProgress,
  source: AgentFoldSource,
): AgentSessionView {
  return updateRun(state, source.runId, (run) => {
    if (run.status !== "running") {
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.progress;run=${run.id};status=${run.status};expected=running`,
      );
    }
    if (source.segmentId !== run.activeSegmentId) {
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.progress;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${run.activeSegmentId ?? "missing"}`,
      );
    }
    return {
      ...run,
      progress: {
        ...run.progress,
        ...(progress.step !== undefined ? { step: progress.step } : {}),
        ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
        ...(progress.usage ? { usage: { ...progress.usage } } : {}),
      },
      ...(statedFootprint(progress.contextTokens) !== null
        ? { contextTokens: progress.contextTokens }
        : {}),
    };
  });
}

export function onRunFinished(
  state: AgentSessionView,
  run: AgentRunFact,
  interrupts: readonly AgentInterrupt[],
  source: AgentFoldSource,
): AgentSessionView {
  const finished = projectFinishedRun(run, source);
  if (isDuplicateRunFinish(state, finished, interrupts)) return state;
  const previous = state.runsById[run.id];
  if (!previous) {
    throw new Error(`agent.fold.runMissing:event=segment.finished;run=${run.id}`);
  }
  if (previous.status !== "running") {
    throw new Error(
      `agent.fold.runStatusMismatch:event=segment.finished;run=${run.id};status=${previous.status};expected=running`,
    );
  }
  if (source.segmentId !== previous.activeSegmentId) {
    throw new Error(
      `agent.fold.segmentMismatch:event=segment.finished;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${previous.activeSegmentId ?? "missing"}`,
    );
  }

  let next = foldRunSnapshot(state, run);
  if (interrupts.length === 0) return next;
  next = mergeRunPendingInterrupts(
    next,
    run.id,
    interrupts.map((interrupt) => ({ itemId: interrupt.itemId, type: interrupt.type })),
  );
  for (const interrupt of interrupts) next = materializeInterrupt(next, interrupt, source);
  return next;
}
