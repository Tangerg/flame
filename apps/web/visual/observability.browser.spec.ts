import { expect, test } from "./test";

for (const event of ["visibilitychange", "pagehide"] as const) {
  test(`observability flushes pending spans on ${event}`, async ({ page }) => {
    await page.route("**/telemetry-probe/v1/**", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
    );
    await page.goto("/visual/?fixture=agent&state=long-content", {
      waitUntil: "domcontentloaded",
    });
    await page.locator("html[data-visual-ready]").waitFor();
    const exported = page.waitForRequest("**/telemetry-probe/v1/traces");
    await page.evaluate(async (event) => {
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
      endSpan(startRunSpan({ fixture: event }));
      if (event === "visibilitychange") {
        Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
      }
      document.dispatchEvent(new Event(event));
    }, event);
    const payload = (await exported).postDataJSON();
    expect(payload.resourceSpans[0].scopeSpans[0].spans[0].name).toBe("agent.run");
    await page.evaluate(async () => {
      await Reflect.get(window, "disposeTelemetryProbe")();
      Reflect.deleteProperty(window, "disposeTelemetryProbe");
      Reflect.deleteProperty(document, "visibilityState");
    });
  });
}
