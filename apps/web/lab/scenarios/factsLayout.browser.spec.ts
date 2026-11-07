import type { TestInfo } from "@playwright/test";
import { expect, test, type Locator, type Page } from "./test";
type FactsGeometry = Awaited<ReturnType<typeof measureFacts>>;

function expectWithinPixel(actual: number, expected: number, message: string) {
  expect(Math.abs(actual - expected), message).toBeLessThanOrEqual(1);
}

async function measureFacts(facts: Locator, record: Locator) {
  const recordBounds = (await record.boundingBox())!;
  const bounds = (await facts.boundingBox())!;
  const pairs = await facts.locator(":scope > div").evaluateAll((nodes) =>
    nodes.map((node) => {
      const box = node.getBoundingClientRect();
      const value = node.querySelector(":scope > dd")!.getBoundingClientRect();
      return {
        label: node.querySelector(":scope > dt")?.textContent ?? "",
        x: box.x,
        y: box.y,
        right: box.right,
        width: box.width,
        valueX: value.x,
        valueRight: value.right,
        overflow: node.scrollWidth - node.clientWidth,
      };
    }),
  );
  return { record: recordBounds, facts: bounds, pairs };
}

function expectReadableFacts({ record, facts, pairs }: FactsGeometry) {
  expect(facts.x).toBeGreaterThanOrEqual(record.x);
  expect(facts.x + facts.width).toBeLessThanOrEqual(record.x + record.width + 1);
  expect(pairs.length).toBeGreaterThan(0);
  const first = pairs[0]!;
  const columns = pairs.filter((pair) => Math.abs(pair.y - first.y) < 1);
  for (const [index, pair] of pairs.entries()) {
    expect(pair.x).toBeGreaterThanOrEqual(facts.x);
    expect(pair.right).toBeLessThanOrEqual(facts.x + facts.width + 1);
    expect(pair.overflow).toBeLessThanOrEqual(1);
    const column = columns[index % columns.length]!;
    expectWithinPixel(pair.width, first.width, `${pair.label} shares the column width`);
    expectWithinPixel(pair.x, column.x, `${pair.label} starts on its column edge`);
    expectWithinPixel(pair.right, column.right, `${pair.label} ends on its column edge`);
    expectWithinPixel(pair.valueX, pair.x, `${pair.label} value starts with its pair`);
    expectWithinPixel(pair.valueRight, pair.right, `${pair.label} value fills its pair`);
    const previous = pairs[index - 1];
    if (!previous) continue;
    if (Math.abs(pair.y - previous.y) < 1) expect(pair.x).toBeGreaterThan(previous.right);
    else expect(pair.y).toBeGreaterThan(previous.y);
  }
}

async function attachFacts(page: Page, testInfo: TestInfo, geometry: unknown) {
  await testInfo.attach("facts-geometry.json", {
    body: JSON.stringify(geometry, null, 2),
    contentType: "application/json",
  });
  await testInfo.attach("facts-page.png", {
    body: await page.screenshot({ animations: "disabled", caret: "hide" }),
    contentType: "image/png",
  });
}

for (const width of [1120, 1472]) {
  test(`accounting facts share equal columns in their reading order ${width}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/lab/fixture.html?fixture=workspace&state=dock-usage&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const panel = page.getByRole("region", { name: "Usage", exact: true });
    const record = panel.getByRole("article").first();
    await record.getByRole("button", { name: "Token and coverage details", exact: true }).click();
    const facts = record.locator("dl");
    await expect(facts.locator("dt")).toHaveText([
      "Known cost (USD)",
      "Usage / attempts",
      "Priced / attempts",
      "Known input tokens",
      "Known output tokens",
      "Known cache read tokens",
      "Known cache write tokens",
      "Known reasoning tokens",
    ]);
    await expect(facts.locator("dd").first()).toHaveText("3.04");
    await facts.locator(":scope > div").first().scrollIntoViewIfNeeded();
    const geometry = await measureFacts(facts, record);
    await attachFacts(page, testInfo, geometry);
    expectReadableFacts(geometry);
  });

  test(`long evidence identities and attribute paths wrap inside equal columns ${width}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/lab/fixture.html?fixture=workspace&state=dock-diagnostics&theme=light&font-size=18",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const trace = `trace_${"0123456789abcdef".repeat(7)}`;
    const path = `/Users/visual/workspace/${"retained-evidence/".repeat(12)}original-request.json`;
    const attribute = "request.context.association.workspace.resolvedReference";
    await page.evaluate(
      async ({ trace, path, attribute }) => {
        const modulePath = "/src/lib/observability/stores.ts";
        const { useTelemetryStore } = await import(modulePath);
        useTelemetryStore.getState().ingestSpans([
          {
            id: "facts-long-values",
            traceId: trace,
            parentSpanId: "span_parent_recorded",
            name: "Inspect retained request evidence",
            kind: "client",
            startMs: 1785484800000,
            durationMillis: 124,
            status: "ok",
            attrs: { [attribute]: path, "model.provider": "openai" },
          },
        ]);
      },
      { trace, path, attribute },
    );
    const panel = page.getByRole("region", { name: "Client diagnostics", exact: true });
    await panel.getByRole("button", { name: "Refresh", exact: true }).click();
    const record = panel
      .getByRole("region", { name: "Local observations", exact: true })
      .locator("section")
      .first();
    await record.getByRole("button", { expanded: false }).first().click();
    const identityFacts = record.locator("dl").first();
    const attributes = record.locator("dl").last();
    const traceValue = identityFacts.locator("dd").filter({ hasText: trace });
    const pathValue = attributes.locator("dd").filter({ hasText: path });
    await expect(traceValue).toHaveText(trace);
    await expect(pathValue).toHaveText(path);
    await expect(attributes.locator("dt")).toHaveText([attribute, "model.provider"]);
    await identityFacts.locator(":scope > div").first().scrollIntoViewIfNeeded();
    const geometry = {
      identity: await measureFacts(identityFacts, record),
      attributes: await measureFacts(attributes, record),
    };
    await attachFacts(page, testInfo, geometry);
    expectReadableFacts(geometry.identity);
    expectReadableFacts(geometry.attributes);
    await attributes.locator(":scope > div").first().scrollIntoViewIfNeeded();
    await expect(pathValue).toBeInViewport();
    await testInfo.attach("attribute-path.png", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    const reading = await pathValue.evaluate(
      (node) =>
        new Promise<{
          width: number;
          height: number;
          visibleWidth: number;
          visibleHeight: number;
          ratio: number;
        }>((resolve) => {
          const observer = new IntersectionObserver(([entry]) => {
            observer.disconnect();
            resolve({
              width: entry!.boundingClientRect.width,
              height: entry!.boundingClientRect.height,
              visibleWidth: entry!.intersectionRect.width,
              visibleHeight: entry!.intersectionRect.height,
              ratio: entry!.intersectionRatio,
            });
          });
          observer.observe(node);
        }),
    );
    await testInfo.attach("attribute-reading.json", {
      body: JSON.stringify(reading, null, 2),
      contentType: "application/json",
    });
    expect(
      reading.width - reading.visibleWidth,
      "the path fits its reading width",
    ).toBeLessThanOrEqual(1);
    expect(
      reading.height - reading.visibleHeight,
      "the complete path is revealed",
    ).toBeLessThanOrEqual(1);
  });
}

test("trajectory identities and a complete timestamp share equal columns", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 1120, height: 900 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-trajectory&theme=light&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const record = page.locator('[data-trajectory-record="model:call_visual_recovered"]');
  await record.getByRole("button", { expanded: false }).click();
  const facts = record.locator("dl");
  await expect(facts.getByText("Record ID", { exact: true })).toBeVisible();
  await expect(facts.getByText("Run ID", { exact: true })).toBeVisible();
  await expect(facts.locator("dd").filter({ hasText: "2026-07-31T08:01:00.000Z" })).toHaveText(
    "2026-07-31T08:01:00.000Z",
  );
  await facts.locator(":scope > div").first().scrollIntoViewIfNeeded();
  const geometry = await measureFacts(facts, record);
  await attachFacts(page, testInfo, geometry);
  expectReadableFacts(geometry);
});
