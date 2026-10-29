import { describe, expect, it, vi } from "vitest";
import {
  startObservability,
  type ObservabilityTeardown,
} from "@/features/observability/observabilityLifecycle";
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("startObservability", () => {
  it("reports initialization failures without breaking plugin startup", async () => {
    const error = new Error("otel failed");
    const reportFailure = vi.fn();

    startObservability(() => Promise.reject(error), reportFailure);
    await tick();

    expect(reportFailure).toHaveBeenCalledWith(error);
  });

  it("runs teardown after initialization", async () => {
    const teardown = vi.fn();
    const dispose = startObservability(() => Promise.resolve(teardown), vi.fn());
    await tick();

    await dispose();

    expect(teardown).toHaveBeenCalledOnce();
  });

  it("tears down an initializer that resolves after disposal", async () => {
    const teardown = vi.fn<ObservabilityTeardown>();
    const initialization = Promise.withResolvers<ObservabilityTeardown>();
    const dispose = startObservability(() => initialization.promise, vi.fn());

    dispose();
    initialization.resolve(teardown);
    await tick();

    expect(teardown).toHaveBeenCalledOnce();
  });
});

it("observes rejected teardown and joins repeated disposal", async () => {
  const failure = new Error("flush failed");
  const teardown = vi.fn(() => Promise.reject(failure));
  const reportFailure = vi.fn();
  const dispose = startObservability(() => Promise.resolve(teardown), reportFailure);
  await tick();
  await Promise.all([dispose(), dispose()]);
  expect(teardown).toHaveBeenCalledOnce();
  expect(reportFailure).toHaveBeenCalledExactlyOnceWith(failure);
});

it("reports a synchronous initializer failure", async () => {
  const failure = new Error("setup refused");
  const reportFailure = vi.fn();
  await startObservability(() => {
    throw failure;
  }, reportFailure)();
  expect(reportFailure).toHaveBeenCalledExactlyOnceWith(failure);
});
