// A LIVE WINDOW, not a telemetry database: durable telemetry leaves the device via OTLP.
// In-memory only — localStorage blocks the main thread on every write at this volume, and
// IndexedDB buys nothing for live triage.
//
// Spans and logs are append streams, so each is a bounded ring buffer; metrics are
// CUMULATIVE and bounded by attribute cardinality, so they are keyed rows instead.

import type { ResourceMetrics } from "@opentelemetry/sdk-metrics";
import { create } from "zustand";

const SPAN_CAP = 500;
const LOG_CAP = 1000;

// Inlined rather than imported so this module does not pull the metrics SDK into the
// static graph — ./setup dynamic-imports it, keeping it off the first-paint path.
const HISTOGRAM_DATA_POINT = 0;

type Attrs = Record<string, string | number | boolean>;

export type InstrumentKind = "histogram" | "counter";

/** One metric row per (instrument name, attribute combo). */
export interface MetricRow {
  id: string;
  name: string;
  unit: string;
  description: string;
  kind: InstrumentKind;
  attrs: Attrs;
  count: number;
  sum: number;
  p50?: number;
  p95?: number;
  avg?: number;
}

/** One ended span. Flattened from a ReadableSpan by the sink. */
export interface SpanRow {
  id: string; // spanId
  traceId: string;
  parentSpanId?: string;
  name: string;
  kind: string;
  startMs: number; // epoch ms
  durationMillis: number;
  status: "unset" | "ok" | "error";
  /** Status description — for error spans this is the failure message
   *  (endSpan sets it from the thrown error). Absent on ok/unset spans. */
  statusMessage?: string;
  attrs: Attrs;
}

/** One emitted log record. Flattened from an SdkLogRecord by the sink. */
export interface LogRow {
  id: string; // monotonic local id (records have no stable id)
  timeMs: number; // epoch ms
  severity: string; // "INFO" / "WARN" / …
  body: string;
  // Correlation — filled natively from the active span when the log was
  // emitted, so a log lines up with the run/rpc span it happened inside.
  traceId?: string;
  spanId?: string;
  attrs: Attrs;
}

interface State {
  metrics: Record<string, MetricRow>;
  spans: SpanRow[];
  logs: LogRow[];
  ingestMetrics: (batch: ResourceMetrics) => void;
  ingestSpans: (rows: SpanRow[]) => void;
  ingestLogs: (rows: LogRow[]) => void;
  clear: () => void;
}

export const useTelemetryStore = create<State>((set) => ({
  metrics: {},
  spans: [],
  logs: [],

  // CUMULATIVE temporality → every export carries running totals, so replace
  // wholesale rather than merge (merging would double-count).
  ingestMetrics: (batch) => {
    const next: Record<string, MetricRow> = {};
    for (const scope of batch.scopeMetrics) {
      for (const m of scope.metrics) {
        const isHist = m.dataPointType === HISTOGRAM_DATA_POINT;
        for (const dp of m.dataPoints) {
          const attrs = (dp.attributes ?? {}) as Attrs;
          const id = `${m.descriptor.name}|${stableKey(attrs)}`;
          next[id] = isHist
            ? histogramRow(id, m.descriptor, dp.value as HistogramValue, attrs)
            : counterRow(id, m.descriptor, dp.value as number, attrs);
        }
      }
    }
    set({ metrics: next });
  },

  // Append + clamp to the newest SPAN_CAP. The sink hands a whole batch so
  // this runs once per flush, not once per span.
  ingestSpans: (rows) =>
    set((s) => {
      if (rows.length === 0) return s;
      const merged = s.spans.concat(rows);
      return { spans: merged.length > SPAN_CAP ? merged.slice(merged.length - SPAN_CAP) : merged };
    }),

  ingestLogs: (rows) =>
    set((s) => {
      if (rows.length === 0) return s;
      const merged = s.logs.concat(rows);
      return { logs: merged.length > LOG_CAP ? merged.slice(merged.length - LOG_CAP) : merged };
    }),

  clear: () => set({ metrics: {}, spans: [], logs: [] }),
}));

interface HistogramValue {
  count: number;
  sum?: number;
  buckets: { boundaries: number[]; counts: number[] };
}

// MetricDescriptor is structurally `{ name, unit, description }` for our use;
// avoid importing the SDK type here to keep this module SDK-free.
interface Descriptor {
  name: string;
  unit: string;
  description: string;
}

function histogramRow(id: string, desc: Descriptor, v: HistogramValue, attrs: Attrs): MetricRow {
  const sum = v.sum ?? 0;
  const { p50, p95 } = estimatePercentiles(v.buckets);
  return {
    id,
    name: desc.name,
    unit: desc.unit,
    description: desc.description,
    kind: "histogram",
    attrs,
    count: v.count,
    sum,
    p50,
    p95,
    avg: v.count > 0 ? sum / v.count : 0,
  };
}

function counterRow(id: string, desc: Descriptor, total: number, attrs: Attrs): MetricRow {
  return {
    id,
    name: desc.name,
    unit: desc.unit,
    description: desc.description,
    kind: "counter",
    attrs,
    count: total,
    sum: total,
  };
}

// Deterministic attribute key so "a=1,b=2" and "b=2,a=1" collapse to one row.
function stableKey(attrs: Attrs): string {
  return Object.entries(attrs)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, v]) => `${k}=${String(v)}`)
    .join(",");
}

// Returns the upper boundary of the bucket the percentile lands in, so it errs HIGH — except
// in the unbounded overflow bucket, which has no upper boundary and so under-reports the
// tail. The histogram keeps counts rather than observations, so this cannot be made exact:
// fine for a dev "is this slow?" eyeball, not an SLO number.
function estimatePercentiles(buckets: HistogramValue["buckets"]): { p50: number; p95: number } {
  const total = buckets.counts.reduce((a, b) => a + b, 0);
  if (total === 0) return { p50: 0, p95: 0 };
  const t50 = total * 0.5;
  const t95 = total * 0.95;
  let running = 0;
  let p50 = 0;
  let p50Done = false;
  for (let i = 0; i < buckets.counts.length; i++) {
    running += buckets.counts[i]!;
    if (!p50Done && running >= t50) {
      p50 = buckets.boundaries[i] ?? buckets.boundaries.at(-1) ?? 0;
      p50Done = true;
    }
    if (running >= t95) {
      return { p50, p95: buckets.boundaries[i] ?? buckets.boundaries.at(-1) ?? 0 };
    }
  }
  const last = buckets.boundaries.at(-1) ?? 0;
  // Use p50Done to avoid the falsy-value trap: p50 could legitimately be 0
  // (the first bucket boundary), but `p50 || last` would substitute `last`.
  return { p50: p50Done ? p50 : last, p95: last };
}
