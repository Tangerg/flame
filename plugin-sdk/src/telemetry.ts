export type TelemetryAttributes = Record<string, string | number | boolean>;

type InstrumentKind = "histogram" | "counter";

export interface MetricRow {
  id: string;
  name: string;
  unit: string;
  description: string;
  kind: InstrumentKind;
  attrs: TelemetryAttributes;
  count: number;
  sum: number;
  p50?: number;
  p95?: number;
  avg?: number;
}

export interface SpanRow {
  id: string;
  traceId: string;
  parentSpanId?: string;
  name: string;
  kind: string;
  startMs: number;
  durationMillis: number;
  status: "unset" | "ok" | "error";
  statusMessage?: string;
  attrs: TelemetryAttributes;
}

export interface LogRow {
  id: string;
  timeMs: number;
  severity: string;
  body: string;
  traceId?: string;
  spanId?: string;
  attrs: TelemetryAttributes;
}

export interface TelemetrySnapshot {
  spans: SpanRow[];
  metrics: MetricRow[];
  logs: LogRow[];
}

// Reads are detached from the client buffer. Both operations refuse after the
// component lifetime ends; clearing never changes Runtime accounting.
export interface ClientTelemetry {
  read(): TelemetrySnapshot;
  clear(): void;
}
