import { type AgentDependencies } from "@/features/agent/application/agentDependencies";
import { type FlameClient } from "@flame/runtime-client";
import { asRunId, asSegmentId } from "@flame/runtime-client";
import { type AgentRunView, type AgentSessionView } from "@/features/agent/public/sessionView";
import { refreshAgentSessionProjection } from "@/features/agent/application/session/refreshSessionProjection";

import { type RunStream } from "@/features/agent/adapters/agentRunPump";
import { retireRunStream } from "@/features/agent/adapters/runStreamOpening";
import { snapshotRunStream } from "@/features/agent/adapters/snapshotRunStream";
interface AgentSessionRecoveryOptions {
  client: Pick<FlameClient, "runs">;
  sessionId: string;
  signal: AbortSignal;
  isCancelled: () => boolean;
  hasInteracted: () => boolean;
  isFollowing: (runId: string, segmentId: string) => boolean;
  setAbortController: (controller: AbortController) => void;
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
  onSynchronizationFailed: (error: unknown) => void;
}

export function startAgentSessionRecovery(
  agentAccess: AgentDependencies,
  options: AgentSessionRecoveryOptions,
): Promise<AgentSessionView | null> {
  return recover(agentAccess, options).catch((error: unknown) => {
    if (!options.signal.aborted && !options.isCancelled()) options.onSynchronizationFailed(error);
    return null;
  });
}

function stale(options: AgentSessionRecoveryOptions): boolean {
  return options.signal.aborted || options.isCancelled() || options.hasInteracted();
}

async function recover(
  agentAccess: AgentDependencies,
  options: AgentSessionRecoveryOptions,
): Promise<AgentSessionView | null> {
  const view = await refreshAgentSessionProjection(agentAccess, options.sessionId, {
    canCommit: () => !stale(options),
    signal: options.signal,
  });
  if (!view || stale(options)) return null;

  const runningRoots = Object.values(view.runsById).filter(
    (run) => run.parentRunId === null && run.status === "running",
  );
  if (runningRoots.length > 1) {
    throw new Error(
      `cannot synchronize agent session ${options.sessionId}: durable snapshot contains ${runningRoots.length} running root runs (${runningRoots.map((run) => run.id).join(", ")}); expected at most one`,
    );
  }
  const root = runningRoots[0];
  if (root) {
    void attachRootRun(agentAccess, options, root).catch(ignorePumpReportedFailure);
  }
  return view;
}

async function attachRootRun(
  agentAccess: AgentDependencies,
  options: AgentSessionRecoveryOptions,
  run: AgentRunView,
): Promise<void> {
  const segmentId = run.activeSegmentId;
  if (!segmentId || options.isFollowing(run.id, segmentId)) return;

  const controller = new AbortController();
  const abort = () => controller.abort(options.signal.reason);
  if (options.signal.aborted) abort();
  else options.signal.addEventListener("abort", abort, { once: true });
  options.setAbortController(controller);
  try {
    let stream: Awaited<ReturnType<typeof options.client.runs.subscribe>>;
    try {
      const opened = await snapshotRunStream(
        agentAccess,
        options.client,
        options.sessionId,
        run.id,
        segmentId,
        controller.signal,
        () => !stale(options) && !controller.signal.aborted,
      );
      if (!opened) return;
      stream = opened;
    } catch (error) {
      if (options.isCancelled() || controller.signal.aborted) return;
      if (agentAccess.runtime.isRunGone(error)) {
        await refreshAgentSessionProjection(agentAccess, options.sessionId, {
          canCommit: () => !stale(options),
          signal: options.signal,
        }).catch((readError: unknown) => {
          if (options.isCancelled() || controller.signal.aborted) return;
          options.onSynchronizationFailed(readError);
        });
        return;
      }
      options.onSynchronizationFailed(error);
      return;
    }
    if (options.isCancelled() || controller.signal.aborted) {
      retireRunStream(stream);
      return;
    }
    await options.pump(
      {
        result: {
          runId: asRunId(stream.result.runId),
          segmentId: asSegmentId(stream.result.segmentId),
          ...(stream.result.headEventId ? { headEventId: stream.result.headEventId } : {}),
        },
        events: stream.events,
      },
      controller.signal,
    );
  } finally {
    options.signal.removeEventListener("abort", abort);
  }
}

function ignorePumpReportedFailure(): void {}
