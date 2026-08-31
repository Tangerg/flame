// A live window, not a database. Spans and logs are append streams, so each is a bounded
// ring buffer; metrics are CUMULATIVE and bounded by attribute cardinality, so keyed rows.

import type { ResourceMetrics } from "@opentelemetry/sdk-metrics";
import { create } from "zustand";

const SPAN_CAP = 500;
const LOG_CAP = 1000;

// Inlined, not imported: keeps the metrics SDK out of this module's static graph.
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
  /** For error spans, the failure message. Absent on ok/unset spans. */
  statusMessage?: string;
  attrs: Attrs;
}

/** One emitted log record. Flattened from an SdkLogRecord by the sink. */
export interface LogRow {
  id: string; // monotonic local id (records have no stable id)
  timeMs: number; // epoch ms
  severity: string; // "INFO" / "WARN" / …
  body: string;
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

  // CUMULATIVE temporality: every export carries running totals, so replace rather than
  // merge — merging double-counts.
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

// The bucket's upper boundary, so it errs HIGH — except in the unbounded overflow bucket,
// which under-reports the tail. Counts, not observations: cannot be made exact.
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
  // p50Done, not `p50 || last`: p50 can legitimately be 0 (the first bucket boundary).
  return { p50: p50Done ? p50 : last, p95: last };
}
