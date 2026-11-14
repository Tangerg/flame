import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

test("hidden flushes pending spans before the batch timer; pagehide alone does not", async ({
  page,
}) => {
  const exports: unknown[] = [];
  const timerEpoch = new Date();
  await page.clock.install({ time: timerEpoch });
  await page.route("**/telemetry-probe/v1/**", async (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/traces"))
      exports.push(route.request().postDataJSON());
    await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
  });
  await page.goto("/lab/fixture.html?fixture=agent&state=long-content", {
    waitUntil: "domcontentloaded",
  });
  await expectLabReady(page);
  // The scene owns a fixed calendar Date; pause the timer clock from its runner epoch.
  await page.clock.pauseAt(new Date(timerEpoch.getTime() + 60_000));
  await page.evaluate(async () => {
    const setupPath = "/src/lib/observability/setup.ts";
    const tracingPath = "/src/lib/observability/tracing.ts";
    const [{ setupObservability }, { startRunSpan, endSpan }] = await Promise.all([
      import(setupPath),
      import(tracingPath),
    ]);
    const dispose = await setupObservability({
      serviceName: "browser-flush",
      serviceVersion: "1",
      otlpEndpoint: `${location.origin}/telemetry-probe`,
    });
    Reflect.set(window, "disposeTelemetryProbe", dispose);
    endSpan(startRunSpan({ fixture: "visibilitychange" }));
  });
  try {
    await page.clock.runFor(4_999);
    expect(exports).toEqual([]);
    await page.evaluate(() =>
      window.dispatchEvent(new PageTransitionEvent("pagehide", { persisted: true })),
    );
    expect(exports).toEqual([]);
    await page.evaluate(() => {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
      document.dispatchEvent(new Event("visibilitychange"));
    });
    // The clock stays paused at 4,999 ms: the five-second batch cannot satisfy this assertion.
    await expect.poll(() => exports.length).toBe(1);
    expect(exports[0]).toMatchObject({
      resourceSpans: [{ scopeSpans: [{ spans: [{ name: "agent.run" }] }] }],
    });
  } finally {
    await page.evaluate(async () => {
      await Reflect.get(window, "disposeTelemetryProbe")();
      Reflect.deleteProperty(window, "disposeTelemetryProbe");
      Reflect.deleteProperty(document, "visibilityState");
    });
  }
});
