// Instruments are bound LATE, not at module load: the metrics API has no proxy meter, so
// an instrument created before a MeterProvider is registered is a NoopInstrument FOREVER —
// a later `setGlobalMeterProvider` does not upgrade it. This module loads very early (the
// reducer imports it), so `lib/observability/setup` calls `bindMetricInstruments()` right
// after registering the provider. Until then `measure*` are cheap no-ops.

import type { Counter, Histogram } from "@opentelemetry/api";
import { metrics } from "@opentelemetry/api";

interface Instruments {
  reducer: Histogram;
  shiki: Histogram;
  mermaid: Histogram;
  events: Counter;
}

let inst: Instruments | null = null;

/** Create the instruments against the (now-registered) global MeterProvider.
 *  Called once by lib/observability/setup after setGlobalMeterProvider. */
export function bindMetricInstruments(): void {
  const meter = metrics.getMeter("flame");
  inst = {
    reducer: meter.createHistogram("flame.reducer.duration", {
      description: "Time spent reducing one StreamEvent",
      unit: "ms",
    }),
    shiki: meter.createHistogram("flame.shiki.highlight.duration", {
      description: "Time spent highlighting one code block",
      unit: "ms",
    }),
    mermaid: meter.createHistogram("flame.mermaid.render.duration", {
      description: "Time spent rendering one mermaid diagram",
      unit: "ms",
    }),
    events: meter.createCounter("flame.run.event.count", {
      description: "Number of run StreamEvents processed",
    }),
  };
}

/**
 * Wrap one synchronous reducer call. Records duration + bumps the StreamEvent
 * counter, both tagged with `eventType`. Re-throws on error so the reducer's
 * existing error path keeps working. No-op until instruments are bound.
 */
export function measureReduce<T>(eventType: string, fn: () => T): T {
  const start = performance.now();
  try {
    return fn();
  } finally {
    if (inst) {
      inst.reducer.record(performance.now() - start, { eventType });
      inst.events.add(1, { eventType });
    }
  }
}

export function measureShikiHighlight(ms: number, lang: string): void {
  inst?.shiki.record(ms, { lang });
}

export function measureMermaidRender(ms: number): void {
  inst?.mermaid.record(ms);
}
