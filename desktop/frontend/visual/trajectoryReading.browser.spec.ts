import { READING_BLOCKS_MARKDOWN } from "./agentSessionSnapshots";
import { expect, test } from "./test";

test("expanded record facts retain shared column edges", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-trajectory&theme=light&font-size=14");
  await page.locator("html[data-visual-ready]").waitFor();
  const record = page.locator('[data-trajectory-record="model:call_visual_recovered"]');
  await record.getByRole("button").first().click();
  const facts = record.locator("dl");
  const geometry = await facts.evaluate((list) =>
    [...list.children].map((fact) => {
      const bounds = fact.getBoundingClientRect();
      return { left: bounds.left, width: bounds.width };
    }),
  );
  expect(geometry).toHaveLength(7);
  expect(new Set(geometry.map((fact) => fact.width)).size).toBe(1);
  expect(new Set(geometry.map((fact) => fact.left)).size).toBe(2);
  for (const [index, fact] of geometry.entries()) {
    expect(fact.left).toBe(geometry[index % 2]!.left);
  }

  await page.evaluate(() => {
    document.documentElement.dir = "rtl";
  });
  const mirrored = await facts.evaluate((list) =>
    [...list.children].map((fact) => fact.getBoundingClientRect().right),
  );
  expect(new Set(mirrored).size).toBe(2);
  expect(mirrored[0]).toBeGreaterThan(mirrored[1]!);
  for (const [index, edge] of mirrored.entries()) {
    expect(edge).toBe(mirrored[index % 2]);
  }
});

test("a narrow full view keeps complete fact pairs in one readable column", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await page.goto(
    "/visual/?fixture=workspace&state=full-view&full-view=package:visual:trajectory&theme=dark&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  const record = page.locator('[data-trajectory-record="model:call_visual_recovered"]');
  await record.getByRole("button").first().click();
  const facts = record.locator("dl");
  await expect(facts.locator("dt")).toHaveText([
    "Record ID",
    "Run ID",
    "Segment ID",
    "Recorded settlement",
    "First output (ms)",
    "Input tokens",
    "Output tokens",
  ]);
  await expect(facts.locator("dd")).toHaveText([
    "call_visual_recovered",
    "run_root",
    "segment_visual",
    "2026-07-31T08:01:00.000Z",
    "Unrecorded",
    "Unrecorded",
    "Unrecorded",
  ]);
  const reading = await facts.evaluate((list) => ({
    edges: [...list.children].map((fact) => fact.getBoundingClientRect().left),
    clipped: [...list.querySelectorAll("dt, dd")]
      .filter((node) => node.scrollWidth > node.clientWidth)
      .map((node) => node.textContent),
  }));
  expect(new Set(reading.edges).size).toBe(1);
  expect(reading.clipped).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("record hover marks its summary without tinting the expanded reading body", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-trajectory&theme=light&font-size=14");
  await page.locator("html[data-visual-ready]").waitFor();
  const record = page.locator('[data-trajectory-record="model:call_visual_recovered"]');
  const trigger = record.getByRole("button").first();
  await trigger.click();
  await page.mouse.move(0, 0);
  const background = () => record.evaluate((node) => getComputedStyle(node).backgroundColor);
  const idle = await background();
  await record.locator("dt").first().hover();
  await expect.poll(background).toBe(idle);

  const summaryIdle = await trigger.evaluate((node) => getComputedStyle(node).backgroundColor);
  await trigger.hover();
  await expect
    .poll(() => trigger.evaluate((node) => getComputedStyle(node).backgroundColor))
    .not.toBe(summaryIdle);
});

test("narrow Run metadata keeps its complete timestamp on one line", async ({ page }) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-runs&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
  const run = dock.locator('[data-trajectory-kind="run"]').first();
  await expect(run).toBeAttached();
  const resizer = page.locator(".agent-pane-resizer");
  await resizer.focus();
  await resizer.press("Home");
  await expect.poll(() => dock.evaluate((node) => node.getBoundingClientRect().width)).toBe(320);
  const summary = run.getByRole("button").first();
  await summary.scrollIntoViewIfNeeded();
  const time = summary.locator("time");
  const lines = await time.evaluate((node) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    return [...new Set([...range.getClientRects()].map((rect) => rect.top))];
  });
  expect(lines).toHaveLength(1);
  await expect(summary.locator("code")).toHaveText("run_fan_ok");
  expect(await dock.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
});

test("collapsed Run summaries retain complete identities and their recorded relationships", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 1000 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-runs&agent-state=delegated&theme=light&font-size=14",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
  const runs = dock.locator('[data-trajectory-kind="run"]');
  const identities = await runs.locator("button code").allTextContents();
  expect(identities).toContain("run_root");
  expect(identities).toContain("run_child");
  expect(new Set(identities).size).toBe(await runs.count());

  const child = dock.locator('[data-trajectory-record="run:run_child"]');
  const trigger = child.getByRole("button").first();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  await expect(trigger).toContainText("run_child");
  await trigger.click();
  const facts = child.locator("dl");
  await expect(facts.getByText("Record ID", { exact: true })).toHaveCount(0);
  await expect(facts.getByText("Run ID", { exact: true })).toHaveCount(1);
  await expect(
    facts
      .locator("dt")
      .filter({ hasText: /^Parent Run ID$/ })
      .locator("+ dd"),
  ).toHaveText("run_root");
  await expect(child.getByRole("region", { name: "Recorded evidence", exact: true })).toContainText(
    '"spawnedByItemId": "item_delegate"',
  );
  expect(await dock.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
});

test("a narrow trajectory reads literal multiline text before optional complete JSON", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-trajectory&agent-state=prose-blocks&theme=dark&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
  const record = dock.locator('[data-trajectory-record="item:item_reading_blocks"]');
  await record.getByRole("button").first().click();
  const body = record.locator("p").filter({ hasText: "## Reading notes" });
  await expect(body).toBeVisible();
  expect(await body.textContent()).toBe(READING_BLOCKS_MARKDOWN);
  const reading = await body.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      lineBreaks: style.whiteSpace,
      selectable: style.userSelect,
      overflow: node.scrollWidth - node.clientWidth,
    };
  });
  expect(reading).toEqual({ lineBreaks: "pre-wrap", selectable: "text", overflow: 0 });
  await expect(record.getByRole("region", { name: "Recorded evidence", exact: true })).toHaveCount(
    0,
  );

  await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
  const raw = record.getByRole("region", { name: "Recorded evidence", exact: true });
  await expect(raw).toContainText(JSON.stringify(READING_BLOCKS_MARKDOWN));
  expect(JSON.parse((await raw.textContent())!)).toMatchObject({
    type: "item",
    item: { id: "item_reading_blocks", content: [{ type: "text", text: READING_BLOCKS_MARKDOWN }] },
  });
  expect(await dock.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

test("a large-font Dock keeps page actions on the search row and filters together", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 900 });
  await page.goto("/visual/?fixture=workspace&state=dock-trajectory&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  const controls = page.getByRole("group", { name: "Trajectory controls", exact: true });
  const search = (await controls
    .getByRole("searchbox", { name: "Filter this page" })
    .boundingBox())!;
  const refresh = (await controls.getByRole("button", { name: "Refresh" }).boundingBox())!;
  const kind = (await controls.getByRole("combobox", { name: "Observation type" }).boundingBox())!;
  const run = (await controls.getByRole("combobox", { name: "Run filter" }).boundingBox())!;

  expect(refresh.y + refresh.height / 2).toBeGreaterThan(search.y);
  expect(refresh.y + refresh.height / 2).toBeLessThan(search.y + search.height);
  expect(refresh.x).toBeGreaterThan(search.x + search.width);
  expect(run.y).toBe(kind.y);
  expect(kind.y).toBeGreaterThan(search.y + search.height);
  expect(await controls.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
});
