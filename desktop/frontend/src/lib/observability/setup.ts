// The one place all three OTel signals are wired onto the global providers.

import { metrics } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";
import {
  CompositePropagator,
  W3CBaggagePropagator,
  W3CTraceContextPropagator,
} from "@opentelemetry/core";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { bindMetricInstruments } from "@/lib/metrics";
import type { IMetricReader } from "@opentelemetry/sdk-metrics";
import { MeterProvider, PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics";
import type { LogRecordProcessor } from "@opentelemetry/sdk-logs";
import { BatchLogRecordProcessor, LoggerProvider } from "@opentelemetry/sdk-logs";
import type { SpanProcessor } from "@opentelemetry/sdk-trace-web";
import { BatchSpanProcessor, WebTracerProvider } from "@opentelemetry/sdk-trace-web";
import { LocalLogProcessor, LocalMetricExporter, LocalSpanProcessor } from "./sink";

export interface ObservabilityOptions {
  serviceName: string;
  serviceVersion: string;
  /** OTLP/HTTP base URL. When set, all three signals are ALSO exported there. */
  otlpEndpoint?: string;
}

const LOCAL_METRIC_INTERVAL_MS = 500;

let shutdownFn: (() => Promise<void>) | null = null;

export async function setupObservability(opts: ObservabilityOptions): Promise<void> {
  if (shutdownFn) return; // idempotent — one install per session

  const resource = resourceFromAttributes({
    "service.name": opts.serviceName,
    "service.version": opts.serviceVersion,
  });

  const otlp = opts.otlpEndpoint ? await loadOtlp(opts.otlpEndpoint) : null;

  // ── Traces ──────────────────────────────────────────────────────────────
  const spanProcessors: SpanProcessor[] = [new LocalSpanProcessor()];
  if (otlp) spanProcessors.push(otlp.spanProcessor);
  const tracerProvider = new WebTracerProvider({ resource, spanProcessors });
  tracerProvider.register({
    propagator: new CompositePropagator({
      propagators: [new W3CTraceContextPropagator(), new W3CBaggagePropagator()],
    }),
  });

  // ── Metrics ─────────────────────────────────────────────────────────────
  const readers: IMetricReader[] = [
    new PeriodicExportingMetricReader({
      exporter: new LocalMetricExporter(),
      exportIntervalMillis: LOCAL_METRIC_INTERVAL_MS,
    }),
  ];
  if (otlp) readers.push(otlp.metricReader);
  const meterProvider = new MeterProvider({ resource, readers });
  metrics.setGlobalMeterProvider(meterProvider);
  // The metrics API has no proxy meter: instruments created before registration are a
  // permanent no-op, so lib/metrics builds them here rather than at module load.
  bindMetricInstruments();

  // ── Logs ────────────────────────────────────────────────────────────────
  const logProcessors: LogRecordProcessor[] = [new LocalLogProcessor()];
  if (otlp) logProcessors.push(otlp.logProcessor);
  const loggerProvider = new LoggerProvider({ resource, processors: logProcessors });
  logs.setGlobalLoggerProvider(loggerProvider);

  shutdownFn = async () => {
    await Promise.allSettled([
      tracerProvider.shutdown(),
      meterProvider.shutdown(),
      loggerProvider.shutdown(),
    ]);
    shutdownFn = null;
  };
}

export async function teardownObservability(): Promise<void> {
  await shutdownFn?.();
}

interface OtlpBundle {
  spanProcessor: SpanProcessor;
  metricReader: IMetricReader;
  logProcessor: LogRecordProcessor;
}

// Batch processors, not simple/sync: high volume must not become one network call per span.
async function loadOtlp(endpoint: string): Promise<OtlpBundle> {
  const base = endpoint.replace(/\/$/, "");
  // Only the exporter packages are dynamic; the Batch*Processor wrappers are already in
  // this chunk, so importing them dynamically would be a no-op split.
  const [traceExp, metricExp, logExp] = await Promise.all([
    import("@opentelemetry/exporter-trace-otlp-http"),
    import("@opentelemetry/exporter-metrics-otlp-http"),
    import("@opentelemetry/exporter-logs-otlp-http"),
  ]);
  return {
    spanProcessor: new BatchSpanProcessor(
      new traceExp.OTLPTraceExporter({ url: `${base}/v1/traces` }),
    ),
    metricReader: new PeriodicExportingMetricReader({
      exporter: new metricExp.OTLPMetricExporter({ url: `${base}/v1/metrics` }),
      exportIntervalMillis: 10_000,
    }),
    logProcessor: new BatchLogRecordProcessor({
      exporter: new logExp.OTLPLogExporter({ url: `${base}/v1/logs` }),
    }),
  };
}
