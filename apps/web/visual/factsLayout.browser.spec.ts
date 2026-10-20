import { expect, test, type Locator } from "./test";
async function expectReadableFacts(facts: Locator, record: Locator) {
  const bounds = (await facts.boundingBox())!;
  const recordBounds = (await record.boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(recordBounds.x);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(recordBounds.x + recordBounds.width + 1);
  const cells = await facts.locator(":scope > div").evaluateAll((nodes) =>
    nodes.map((node) => {
      const box = node.getBoundingClientRect();
      return {
        x: box.x,
        y: box.y,
        right: box.right,
        overflow: node.scrollWidth - node.clientWidth,
      };
    }),
  );
  for (const [index, cell] of cells.entries()) {
    expect(cell.x).toBeGreaterThanOrEqual(bounds.x);
    expect(cell.right).toBeLessThanOrEqual(bounds.x + bounds.width + 1);
    expect(cell.overflow).toBeLessThanOrEqual(1);
    const previous = cells[index - 1];
    if (!previous) continue;
    if (Math.abs(cell.y - previous.y) < 1) expect(cell.x).toBeGreaterThan(previous.right);
    else expect(cell.y).toBeGreaterThan(previous.y);
  }
}

for (const width of [1120, 1472]) {
  test(`short accounting facts share rows and retain their reading order ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/visual/?fixture=workspace&state=dock-usage&theme=light&font-size=18");
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
    const cost = (await facts.getByText("Known cost (USD)", { exact: true }).boundingBox())!;
    const coverage = (await facts.getByText("Usage / attempts", { exact: true }).boundingBox())!;
    expect(coverage.y).toBe(cost.y);
    if (width === 1472) {
      const priced = (await facts.getByText("Priced / attempts", { exact: true }).boundingBox())!;
      expect(priced.y).toBe(cost.y);
    }
    await expectReadableFacts(facts, record);
  });

  test(`long evidence identities and attribute paths retain the available width ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/visual/?fixture=workspace&state=dock-diagnostics&theme=light&font-size=18");
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
    const attributesBox = (await attributes.boundingBox())!;
    const pathBox = (await pathValue.boundingBox())!;
    expect(pathBox.width).toBeGreaterThan(attributesBox.width * 0.9);
    await expectReadableFacts(identityFacts, record);
    await expectReadableFacts(attributes, record);
  });
}

test("trajectory identities share space without crowding a complete timestamp", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-trajectory&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  const record = page.locator('[data-trajectory-record="model:call_visual_recovered"]');
  await record.getByRole("button", { expanded: false }).click();
  const facts = record.locator("dl");
  const recordId = (await facts.getByText("Record ID", { exact: true }).boundingBox())!;
  const runId = (await facts.getByText("Run ID", { exact: true }).boundingBox())!;
  expect(runId.y).toBe(recordId.y);
  await expect(facts.locator("dd").filter({ hasText: "2026-07-31T08:01:00.000Z" })).toHaveText(
    "2026-07-31T08:01:00.000Z",
  );
  await expectReadableFacts(facts, record);
});
