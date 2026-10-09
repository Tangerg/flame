import type { AgentInterrupt, AgentRunFact } from "@/plugins/sdk";
import type {
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunProgress,
  AgentRunView,
  AgentSessionView,
  RunUsage,
} from "@/plugins/sdk/types/agentSessionView";
import { dropRunPendingInterrupts, mergeRunPendingInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";
import { foldRunSnapshot } from "./runSnapshot";
import type { AgentFoldSource } from "./source";
import { projectFinishedRun, projectStartedRun } from "../view/runProjection";
import { isAgentRunFailure } from "../view/runOutcome";

function sameRunUsage(left: RunUsage | undefined, right: RunUsage | undefined): boolean {
  if (!left || !right) return left === right;
  return (
    left.inputTokens === right.inputTokens &&
    left.outputTokens === right.outputTokens &&
    left.cacheReadTokens === right.cacheReadTokens &&
    left.costUsd === right.costUsd
  );
}

function sameRunMetrics(left: AgentRunMetrics, right: AgentRunMetrics): boolean {
  return (
    left.steps === right.steps &&
    left.activeDurationMillis === right.activeDurationMillis &&
    sameRunUsage(left.usage, right.usage)
  );
}

function sameRunOutcome(left: AgentRunOutcome | null, right: AgentRunOutcome): boolean {
  if (!left || left.type !== right.type) return false;
  const leftEffects = left.unresolvedEffects ?? [];
  const rightEffects = right.unresolvedEffects ?? [];
  if (
    leftEffects.length !== rightEffects.length ||
    leftEffects.some((effect, index) => {
      const other = rightEffects[index]!;
      return (
        effect.processId !== other.processId ||
        effect.effectId !== other.effectId ||
        effect.cause !== other.cause ||
        effect.reason !== other.reason ||
        effect.detail !== other.detail ||
        effect.output !== other.output
      );
    })
  )
    return false;
  if (left.type === "completed") return true;
  if (isAgentRunFailure(left) && isAgentRunFailure(right)) {
    return (
      left.error.code === right.error.code &&
      left.error.message === right.error.message &&
      left.error.retryAfterSeconds === right.error.retryAfterSeconds
    );
  }
  if (left.type === "canceled" && right.type === left.type) {
    return left.detail === right.detail;
  }
  return false;
}

function isDuplicateRunFinish(
  state: AgentSessionView,
  run: AgentRunView,
  interrupts: readonly AgentInterrupt[],
): boolean {
  const previous = state.runsById[run.id];
  if (!previous || previous.status === "running") return false;
  if (
    previous.status !== run.status ||
    previous.finishedAt !== run.finishedAt ||
    !sameRunMetrics(previous.metrics, run.metrics)
  )
    return false;
  if (run.status === "finished")
    return run.outcome !== null && sameRunOutcome(previous.outcome, run.outcome);
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
    interrupts.map((interrupt) => ({ itemId: interrupt.itemId, kind: interrupt.type })),
  );
  for (const interrupt of interrupts) next = materializeInterrupt(next, interrupt, source);
  return next;
}
