import { trace } from "@opentelemetry/api";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setupObservability } from "@/lib/observability/setup";
import { useTelemetryStore } from "@/lib/observability/stores";

const disposals: (() => Promise<void>)[] = [];
beforeEach(() => {
  vi.stubEnv("OTEL_TRACES_SAMPLER", "always_on");
});
afterEach(async () => {
  try {
    await Promise.all(disposals.splice(0).map((dispose) => dispose()));
  } finally {
    useTelemetryStore.getState().clear();
    vi.unstubAllEnvs();
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
