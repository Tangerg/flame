import type { FlameClient } from "@flame/runtime-contract/client";
import { asRunId } from "@flame/runtime-contract/client";
import type { RunEvent, SessionSnapshot } from "@flame/runtime-contract/wire";

export interface ObservationSink {
  snapshot(snapshot: SessionSnapshot): void;
  event(event: RunEvent): void;
}

export async function observeRun(
  client: { runs: Pick<FlameClient["runs"], "get" | "subscribe"> },
  runId: string,
  sink: ObservationSink,
  signal: AbortSignal,
): Promise<void> {
  while (!signal.aborted) {
    const run = await client.runs.get(asRunId(runId), signal);
    if (run.status !== "running" || !run.activeSegmentId) return;
    const attached = await client.runs.subscribe(
      { runId, segmentId: run.activeSegmentId, snapshot: true },
      signal,
    );
    const events = attached.events[Symbol.asyncIterator]();
    try {
      if (!attached.result.snapshot) throw new Error("Runtime omitted the subscribed snapshot");
      if (signal.aborted) return;
      sink.snapshot(attached.result.snapshot);
      for (;;) {
        const next = await events.next();
        if (next.done || signal.aborted) break;
        sink.event(next.value);
      }
    } finally {
      await events.return?.();
    }
  }
}
