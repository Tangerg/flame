import type { FlameClient } from "@flame/runtime-contract/client";
import { asRunId, asSessionId, isErrorType } from "@flame/runtime-contract/client";
import type { RunEvent, SessionSnapshot } from "@flame/runtime-contract/wire";

export interface ObservationSink {
  snapshot(snapshot: SessionSnapshot): void;
  event(event: RunEvent): void;
}

export async function observeRun(
  client: {
    runs: Pick<FlameClient["runs"], "get" | "subscribe">;
    sessions: Pick<FlameClient["sessions"], "snapshot">;
  },
  runId: string,
  sink: ObservationSink,
  signal: AbortSignal,
): Promise<void> {
  while (!signal.aborted) {
    const run = await client.runs.get(asRunId(runId), signal);
    if (run.status !== "running") {
      const snapshot = await client.sessions.snapshot(asSessionId(run.sessionId), true, signal);
      if (!signal.aborted) sink.snapshot(snapshot);
      return;
    }
    const attached = await client.runs
      .subscribe({ runId, segmentId: run.activeSegmentId!, snapshot: true }, signal)
      .catch((error: unknown) => {
        if (
          isErrorType(error, "stale_segment") ||
          isErrorType(error, "run_waiting") ||
          isErrorType(error, "run_finished")
        ) {
          return undefined;
        }
        throw error;
      });
    if (!attached) continue;
    const events = attached.events[Symbol.asyncIterator]();
    try {
      if (!attached.result.snapshot) throw new Error("Runtime omitted the subscribed snapshot");
      if (signal.aborted) return;
      sink.snapshot(attached.result.snapshot);
      while (!signal.aborted) {
        const next = await events.next();
        if (next.done || signal.aborted) break;
        sink.event(next.value);
      }
    } finally {
      await events.return?.();
    }
  }
}

export interface RuntimeChangeSink {
  refresh(): Promise<void>;
  refreshFailed(error: unknown): void;
}

// A failed refresh is one stale read, not a lost subscription: the next change
// reads again, so only the stream ending stops observation.
export async function followRuntimeChanges(
  client: { runtimeEvents: Pick<FlameClient["runtimeEvents"], "subscribe"> },
  sink: RuntimeChangeSink,
  signal: AbortSignal,
): Promise<void> {
  const subscription = await client.runtimeEvents.subscribe(
    { topics: ["sessions.changed", "runs.changed", "interrupts.changed"] },
    signal,
  );
  for await (const _event of subscription.events) {
    if (signal.aborted) return;
    await sink.refresh().catch((error: unknown) => {
      if (!signal.aborted) sink.refreshFailed(error);
    });
  }
}
