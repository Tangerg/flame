import type { AgentRunFact, AgentSegmentOutcome } from "@/plugins/sdk";
import type {
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunProgress,
  AgentRunView,
  AgentSessionView,
  PendingInterrupt,
  RunUsage,
} from "@/plugins/sdk/types/agentSessionView";
import { dropRunPendingInterrupts, mergeRunPendingInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";
import type { AgentFoldSource } from "./source";
import {
  projectRunMetrics,
  projectStartedRun,
  projectTerminalSegmentOutcome,
} from "../view/runProjection";
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
        effect.detail !== other.detail
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
  outcome: AgentSegmentOutcome,
  metrics: AgentRunMetrics,
  source: AgentFoldSource,
): boolean {
  const run = state.runsById[source.runId];
  if (!run) return false;
  const projectedMetrics = projectRunMetrics(metrics);
  if (!sameRunMetrics(run.metrics, projectedMetrics)) return false;
  if (outcome.type === "interrupt") {
    if (run.status !== "waiting") return false;
    const open = new Set(
      state.pendingInterrupts
        .filter((group) => state.runsById[group.runId]?.rootRunId === run.rootRunId)
        .flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId)),
    );
    return outcome.interrupts.every((interrupt) => open.has(interrupt.itemId));
  }
  if (outcome.type === "suspended") return run.status === "waiting";
  return (
    run.status === "finished" &&
    run.finishedAt === source.timestamp &&
    sameRunOutcome(run.outcome, projectTerminalSegmentOutcome(outcome))
  );
}

function statedFootprint(tokens: number | undefined): number | null {
  return tokens !== undefined && tokens > 0 ? tokens : null;
}

function updateRun(
  state: AgentSessionView,
  runId: string,
  eventType: "segment.progress" | "segment.finished",
  update: (run: AgentRunView) => AgentRunView,
): AgentSessionView {
  const run = state.runsById[runId];
  if (!run) {
    throw new Error(`agent.fold.runMissing:event=${eventType};run=${runId}`);
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
  return updateRun(state, source.runId, "segment.progress", (run) => {
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
  outcome: AgentSegmentOutcome,
  metrics: AgentRunMetrics,
  contextTokens: number,
  source: AgentFoldSource,
): AgentSessionView {
  if (isDuplicateRunFinish(state, outcome, metrics, source)) return state;
  let next = updateRun(state, source.runId, "segment.finished", (run) => {
    if (run.status !== "running") {
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.finished;run=${run.id};status=${run.status};expected=running`,
      );
    }
    if (source.segmentId !== run.activeSegmentId) {
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.finished;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${run.activeSegmentId ?? "missing"}`,
      );
    }
    if (outcome.type === "interrupt" || outcome.type === "suspended") {
      return {
        ...run,
        status: "waiting",
        activeSegmentId: null,
        outcome: null,
        metrics: projectRunMetrics(metrics),
        progress: null,
        contextTokens: statedFootprint(contextTokens) ?? run.contextTokens,
      };
    }
    return {
      ...run,
      status: "finished",
      activeSegmentId: null,
      outcome: projectTerminalSegmentOutcome(outcome),
      metrics: projectRunMetrics(metrics),
      progress: null,
      contextTokens: statedFootprint(contextTokens) ?? run.contextTokens,
      finishedAt: source.timestamp,
    };
  });

  if (outcome.type === "suspended") return next;
  if (outcome.type === "interrupt") {
    const byRunId = new Map<string, PendingInterrupt[]>();
    for (const interrupt of outcome.interrupts) {
      const runId = interrupt.runId;
      const pending = byRunId.get(runId) ?? [];
      pending.push({ itemId: interrupt.itemId, kind: interrupt.type });
      byRunId.set(runId, pending);
    }
    for (const [runId, interrupts] of byRunId) {
      next = mergeRunPendingInterrupts(next, runId, interrupts);
    }
    for (const interrupt of outcome.interrupts) {
      const runId = interrupt.runId;
      next = materializeInterrupt(next, interrupt, { ...source, runId });
    }
    return next;
  }

  return dropRunPendingInterrupts(next, source.runId);
}
