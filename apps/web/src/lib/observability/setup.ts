import { context, metrics, propagation, trace } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";
import {
  CompositePropagator,
  W3CBaggagePropagator,
  W3CTraceContextPropagator,
} from "@opentelemetry/core";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { bindMetricInstruments } from "@/lib/metrics";
import { type IMetricReader } from "@opentelemetry/sdk-metrics";
import { MeterProvider, PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics";
import { type LogRecordProcessor } from "@opentelemetry/sdk-logs";
import { BatchLogRecordProcessor, LoggerProvider } from "@opentelemetry/sdk-logs";
import {
  BatchSpanProcessor,
  StackContextManager,
  TracerProvider,
  type SpanProcessor,
} from "@opentelemetry/sdk-trace";
import {
  LocalLogProcessor,
  LocalMetricExporter,
  LocalSpanProcessor,
} from "@/lib/observability/sink";
export interface ObservabilityOptions {
  serviceName: string;
  serviceVersion: string;
  otlpEndpoint?: string;
}

const LOCAL_METRIC_INTERVAL_MS = 500;

class ObservabilitySession {
  readonly #tracer: TracerProvider;
  readonly #meter: MeterProvider;
  readonly #logger: LoggerProvider;
  readonly #unbindInstruments: () => void;
  #closing: Promise<void> | null = null;

  constructor(
    opts: ObservabilityOptions,
    otlp: OtlpBundle | null,
    private readonly release: () => void,
  ) {
    const resource = resourceFromAttributes({
      "service.name": opts.serviceName,
      "service.version": opts.serviceVersion,
    });
    const spanProcessors: SpanProcessor[] = [new LocalSpanProcessor()];
    if (otlp) spanProcessors.push(otlp.spanProcessor);
    this.#tracer = new TracerProvider({ resource, spanProcessors });

    const readers: IMetricReader[] = [
      new PeriodicExportingMetricReader({
        exporter: new LocalMetricExporter(),
        exportIntervalMillis: LOCAL_METRIC_INTERVAL_MS,
      }),
    ];
    if (otlp) readers.push(otlp.metricReader);
    this.#meter = new MeterProvider({ resource, readers });

    const processors: LogRecordProcessor[] = [new LocalLogProcessor()];
    if (otlp) processors.push(otlp.logProcessor);
    this.#logger = new LoggerProvider({ resource, processors });

    trace.setGlobalTracerProvider(this.#tracer);
    context.setGlobalContextManager(new StackContextManager().enable());
    propagation.setGlobalPropagator(
      new CompositePropagator({
        propagators: [new W3CTraceContextPropagator(), new W3CBaggagePropagator()],
      }),
    );
    metrics.setGlobalMeterProvider(this.#meter);
    logs.setGlobalLoggerProvider(this.#logger);
    this.#unbindInstruments = bindMetricInstruments(this.#meter.getMeter("flame"));
  }

  shutdown(): Promise<void> {
    if (this.#closing) return this.#closing;
    this.#unbindInstruments();
    // SDK shutdown leaves the global registrations in place; replacements must
    // not inherit providers whose processors have already been closed.
    trace.disable();
    metrics.disable();
    logs.disable();
    propagation.disable();
    context.disable();
    this.#closing = Promise.allSettled([
      this.#tracer.shutdown(),
      this.#meter.shutdown(),
      this.#logger.shutdown(),
    ])
      .then((results) => {
        const errors = results.flatMap((result) =>
          result.status === "rejected" ? [result.reason] : [],
        );
        if (errors.length) throw new AggregateError(errors, "observability shutdown failed");
      })
      .finally(this.release);
    return this.#closing;
  }
}

let openingSession: Promise<ObservabilitySession> | null = null;

export async function setupObservability(opts: ObservabilityOptions): Promise<() => Promise<void>> {
  const predecessor = openingSession;
  const opening: Promise<ObservabilitySession> = (async () => {
    if (predecessor) {
      const previous = await predecessor.catch(() => null);
      await previous?.shutdown();
    }
    const otlp = opts.otlpEndpoint ? await loadOtlp(opts.otlpEndpoint) : null;
    return new ObservabilitySession(opts, otlp, () => {
      if (openingSession === opening) openingSession = null;
    });
  })();
  openingSession = opening;
  try {
    const session = await opening;
    return () => session.shutdown();
  } catch (error) {
    if (openingSession === opening) openingSession = null;
    throw error;
  }
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
    spanProcessor: new BatchSpanProcessor({
      exporter: new traceExp.OTLPTraceExporter({ url: `${base}/v1/traces` }),
    }),
    metricReader: new PeriodicExportingMetricReader({
      exporter: new metricExp.OTLPMetricExporter({ url: `${base}/v1/metrics` }),
      exportIntervalMillis: 10_000,
    }),
    logProcessor: new BatchLogRecordProcessor({
      exporter: new logExp.OTLPLogExporter({ url: `${base}/v1/logs` }),
    }),
  };
}
