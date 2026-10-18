import { READING_BLOCKS_MARKDOWN } from "./agentSessionSnapshots";
import { expect, test } from "./test";

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
