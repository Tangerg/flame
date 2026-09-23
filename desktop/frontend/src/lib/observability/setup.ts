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
  otlpEndpoint?: string;
}

const LOCAL_METRIC_INTERVAL_MS = 500;

let shutdownFn: (() => Promise<void>) | null = null;

export async function setupObservability(opts: ObservabilityOptions): Promise<void> {
  if (shutdownFn) return;

  const resource = resourceFromAttributes({
    "service.name": opts.serviceName,
    "service.version": opts.serviceVersion,
  });

  const otlp = opts.otlpEndpoint ? await loadOtlp(opts.otlpEndpoint) : null;

  const spanProcessors: SpanProcessor[] = [new LocalSpanProcessor()];
  if (otlp) spanProcessors.push(otlp.spanProcessor);
  const tracerProvider = new WebTracerProvider({ resource, spanProcessors });
  tracerProvider.register({
    propagator: new CompositePropagator({
      propagators: [new W3CTraceContextPropagator(), new W3CBaggagePropagator()],
    }),
  });

  const readers: IMetricReader[] = [
    new PeriodicExportingMetricReader({
      exporter: new LocalMetricExporter(),
      exportIntervalMillis: LOCAL_METRIC_INTERVAL_MS,
    }),
  ];
  if (otlp) readers.push(otlp.metricReader);
  const meterProvider = new MeterProvider({ resource, readers });
  metrics.setGlobalMeterProvider(meterProvider);
  bindMetricInstruments();

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

async function loadOtlp(endpoint: string): Promise<OtlpBundle> {
  const base = endpoint.replace(/\/$/, "");
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
