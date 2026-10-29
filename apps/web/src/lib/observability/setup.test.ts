import { context, metrics, propagation, trace } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";
import { ExportResultCode } from "@opentelemetry/core";
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http";
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { afterEach, expect, it, vi } from "vitest";
import { setupObservability } from "@/lib/observability/setup";
import { useTelemetryStore } from "@/lib/observability/stores";

const disposals: (() => Promise<void>)[] = [];
afterEach(async () => {
  try {
    await Promise.all(disposals.splice(0).map((dispose) => dispose()));
  } finally {
    useTelemetryStore.getState().clear();
    vi.restoreAllMocks();
  }
});

const options = { serviceName: "session-test", serviceVersion: "1" };

it("binds a fresh provider after shutdown", async () => {
  const first = await setupObservability(options);
  disposals.push(first);
  trace.getTracer("fixture").startSpan("first").end();
  await first();

  const second = await setupObservability(options);
  disposals.push(second);
  trace.getTracer("fixture").startSpan("second").end();
  await second();
  expect(useTelemetryStore.getState().spans.map((span) => span.name)).toEqual(["first", "second"]);
});

it("serializes replacement and fences late disposal of the previous owner", async () => {
  const firstOpening = setupObservability(options);
  const secondOpening = setupObservability(options);
  const [first, second] = await Promise.all([firstOpening, secondOpening]);
  disposals.push(first, second);
  await first();
  trace.getTracer("fixture").startSpan("replacement").end();
  await second();
  expect(useTelemetryStore.getState().spans.map((span) => span.name)).toEqual(["replacement"]);
});

it("preserves parent context and W3C propagation within the active session", async () => {
  const dispose = await setupObservability(options);
  disposals.push(dispose);
  const tracer = trace.getTracer("fixture");
  const parent = tracer.startSpan("parent");
  const baggage = propagation.createBaggage({ fixture: { value: "context" } });
  const active = propagation.setBaggage(trace.setSpan(context.active(), parent), baggage);
  const carrier: Record<string, string> = {};
  context.with(active, () => {
    tracer.startSpan("child").end();
    propagation.inject(context.active(), carrier);
  });
  parent.end();
  expect(trace.getSpan(context.active())).toBeUndefined();
  expect(carrier.traceparent).toBe(
    `00-${parent.spanContext().traceId}-${parent.spanContext().spanId}-01`,
  );
  expect(carrier.baggage).toBe("fixture=context");
  await dispose();
  const child = useTelemetryStore.getState().spans.find((span) => span.name === "child");
  expect(child?.parentSpanId).toBe(parent.spanContext().spanId);
  expect(child?.traceId).toBe(parent.spanContext().traceId);
});

it("flushes every signal to both the local sink and OTLP on shutdown", async () => {
  const spanExport = vi
    .spyOn(OTLPTraceExporter.prototype, "export")
    .mockImplementation((_spans, callback) => callback({ code: ExportResultCode.SUCCESS }));
  const logExport = vi
    .spyOn(OTLPLogExporter.prototype, "export")
    .mockImplementation((_records, callback) => callback({ code: ExportResultCode.SUCCESS }));
  const metricExport = vi
    .spyOn(OTLPMetricExporter.prototype, "export")
    .mockImplementation((_metrics, callback) => callback({ code: ExportResultCode.SUCCESS }));
  const dispose = await setupObservability({ ...options, otlpEndpoint: "http://collector.test/" });
  disposals.push(dispose);
  trace.getTracer("fixture").startSpan("exported-span").end();
  logs.getLogger("fixture").emit({ body: "exported-log" });
  metrics.getMeter("fixture").createCounter("exported-count").add(3);
  await dispose();

  expect(spanExport.mock.calls.flatMap(([spans]) => spans.map((span) => span.name))).toEqual([
    "exported-span",
  ]);
  expect(logExport.mock.calls.flatMap(([records]) => records.map((record) => record.body))).toEqual(
    ["exported-log"],
  );
  expect(metricExport).toHaveBeenCalled();
  const local = useTelemetryStore.getState();
  expect(local.spans.map((span) => span.name)).toEqual(["exported-span"]);
  expect(local.logs.map((record) => record.body)).toEqual(["exported-log"]);
  expect(Object.values(local.metrics).some((metric) => metric.name === "exported-count")).toBe(
    true,
  );
});
