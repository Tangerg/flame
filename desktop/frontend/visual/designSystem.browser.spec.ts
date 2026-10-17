import { expect, test, type Locator, type Page } from "./test";

async function open(page: Page, query: string) {
  await page.goto(`/visual/?${query}&font-size=14`);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
}

async function positions(indicator: Locator, axis: "x" | "y") {
  return indicator.evaluate(async (element, axis) => {
    const frames: number[] = [];
    const start = performance.now();
    while (performance.now() - start < 420) {
      frames.push(element.getBoundingClientRect()[axis]);
      await new Promise(requestAnimationFrame);
    }
    return frames;
  }, axis);
}

function hasIntermediate(frames: number[], from: number, to: number) {
  return frames.some((value) => value > Math.min(from, to) + 1 && value < Math.max(from, to) - 1);
}

for (const theme of ["light", "dark"]) {
  test(`one field token reaches every control kind ${theme}`, async ({ page }) => {
    await page.setViewportSize({ width: 1472, height: 1100 });
    await open(page, `fixture=atoms&theme=${theme}`);
    await page.locator("html").evaluate((root) => {
      root.style.setProperty("--field-radius", "3px");
      root.style.setProperty("--field-background", "rgb(245, 240, 232)");
      root.style.setProperty("--field-shadow", "none");
    });
    const controls = [
      page.getByRole("textbox", { name: "Empty", exact: true }),
      page.getByRole("textbox", { name: "Note", exact: true }),
      page.getByRole("searchbox", { name: "Search", exact: true }).locator(".."),
      page.getByRole("combobox", { name: "Memory status", exact: true }),
      page.getByRole("button", { name: "gpt-5 · high", exact: true }),
    ];
    await expect(controls[0]!).toHaveCSS("min-height", "32px");
    await expect(controls[2]!).toHaveCSS("min-height", "32px");
    await expect(controls[3]!).toHaveCSS("min-height", "36px");
    await expect(controls[4]!).toHaveCSS("min-height", "32px");
    for (const control of controls) {
      await expect(control).toHaveCSS("border-top-left-radius", "3px");
      await expect(control).toHaveCSS("background-color", "rgb(245, 240, 232)");
      await expect(control).toHaveCSS("box-shadow", "none");
    }
  });
}

test("layout tokens change page gutters, form padding and field groups together", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1200, height: 1000 });
  await open(
    page,
    "fixture=workspace&state=full-view&full-view=package:visual:schedules&theme=light",
  );
  await page.getByRole("button", { name: "Add schedule", exact: true }).click();
  await page.locator("html").evaluate((root) => {
    root.style.setProperty("--layout-page-gutter", "40px");
    root.style.setProperty("--layout-card-padding", "32px");
    root.style.setProperty("--layout-field-gap", "24px");
    root.style.setProperty("--surface-card-radius", "28px");
  });
  const editor = page.getByRole("region", { name: "Schedule editor", exact: true });
  await expect(editor).toHaveCSS("padding-inline-start", "32px");
  await expect(editor).toHaveCSS("border-top-left-radius", "28px");
  const group = editor.getByRole("group", { name: "Timing and workspace", exact: true });
  await expect(group.locator(":scope > div")).toHaveCSS("row-gap", "24px");
  const panel = page.getByRole("region", { name: "Schedules", exact: true });
  const pageEdge = (await panel.boundingBox())!.x;
  expect(
    (await panel.getByRole("heading", { name: "Schedules", exact: true }).boundingBox())!.x -
      pageEdge,
  ).toBe(40);
});

test("segments slide continuously and reverse from their painted position", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await open(page, "fixture=atoms&theme=light&motion=full");
  const tabs = page.getByRole("tablist", { name: "Diagnostics", exact: true });
  await tabs.scrollIntoViewIfNeeded();
  const trace = tabs.getByRole("tab", { name: "Trace", exact: true });
  const metrics = tabs.getByRole("tab", { name: "Metrics", exact: true });
  const logs = tabs.getByRole("tab", { name: "Logs", exact: true });
  const from = (await trace.boundingBox())!.x;
  const to = (await metrics.boundingBox())!.x;
  await metrics.click();
  const frames = await positions(metrics.locator('[data-slot="item-highlight"]'), "x");
  expect(
    hasIntermediate(frames, from, to),
    "selection must travel through intermediate positions",
  ).toBe(true);
  expect(Math.abs(frames.at(-1)! - to)).toBeLessThan(1);
  await logs.click();
  await page.evaluate(() => new Promise(requestAnimationFrame));
  const painted = (await logs.locator('[data-slot="item-highlight"]').boundingBox())!.x;
  await trace.click();
  const reversed = await positions(trace.locator('[data-slot="item-highlight"]'), "x");
  expect(
    hasIntermediate(reversed, from, painted),
    "an interrupted movement must continue from its current position",
  ).toBe(true);
  expect(Math.abs(reversed.at(-1)! - from)).toBeLessThan(1);
  await expect(trace).toHaveAttribute("aria-selected", "true");
  await expect(tabs.locator('[data-slot="item-highlight"]')).toHaveCount(1);
});

test("navigation rows share the continuous sliding backplate", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await open(page, "fixture=shell&state=populated&theme=light&motion=full");
  const first = page.getByRole("button", { name: /^Refine Runtime protocol/ });
  const second = page.getByRole("button", { name: /^Align desktop shell/ });
  await first.hover();
  await expect(first.locator('[data-slot="item-highlight"]')).toBeVisible();
  const from = (await first.boundingBox())!.y;
  const to = (await second.boundingBox())!.y;
  await second.hover();
  const frames = await positions(second.locator('[data-slot="item-highlight"]'), "y");
  expect(hasIntermediate(frames, from, to)).toBe(true);
  expect(Math.abs(frames.at(-1)! - to)).toBeLessThan(1);
  await expect(second).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
});

test("reduced motion keeps selection accurate without a sliding transform", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await open(page, "fixture=atoms&theme=dark&motion=full");
  const metrics = page
    .getByRole("tablist", { name: "Diagnostics", exact: true })
    .getByRole("tab", { name: "Metrics", exact: true });
  await metrics.click();
  const indicator = metrics.locator('[data-slot="item-highlight"]');
  const reading = indicator.evaluate(async (element) => {
    const tab = element.closest('[role="tab"]')!;
    const frames: { offset: number; transform: string }[] = [];
    const start = performance.now();
    while (performance.now() - start < 420) {
      frames.push({
        offset: element.getBoundingClientRect().x - tab.getBoundingClientRect().x,
        transform: getComputedStyle(element).transform,
      });
      await new Promise(requestAnimationFrame);
    }
    return frames;
  });
  await page.setViewportSize({ width: 960, height: 720 });
  const frames = await reading;
  expect(
    frames.every(({ offset, transform }) => Math.abs(offset) < 1 && transform === "none"),
    `reduced motion stays anchored while the page reflows: ${JSON.stringify(frames)}`,
  ).toBe(true);
});

test("multiline labels keep neighboring fields at their control size", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await open(page, "fixture=atoms&theme=light");
  const form = page.locator('[data-atom="form-alignment"]');
  await form.evaluate((node) => {
    node.style.width = "440px";
  });
  const short = form.getByRole("textbox", { name: "Short form value", exact: true });
  const long = form.getByRole("textbox", { name: "Long form value", exact: true });
  const shortBox = (await short.boundingBox())!;
  const longBox = (await long.boundingBox())!;
  expect(longBox.x).toBeGreaterThan(shortBox.x);
  expect(longBox.y).toBeGreaterThan(shortBox.y);
  expect(shortBox.height).toBe(32);
  expect(longBox.height).toBe(32);
});

test("fields share their text inset and reset native appearance", async ({ page }) => {
  await open(page, "fixture=atoms&theme=light");
  const input = page.getByRole("textbox", { name: "Empty", exact: true });
  const area = page.getByRole("textbox", { name: "Note", exact: true });
  const picker = page.getByRole("button", { name: "gpt-5 · high", exact: true });
  for (const field of [input, area, picker]) {
    await expect(field).toHaveCSS("padding-inline-start", "10px");
    await expect(field).toHaveCSS("appearance", "none");
  }
});

test("one row and gutter token reaches settings, plugins and modal chrome", async ({ page }) => {
  await open(page, "fixture=workspace&state=settings&pane=providers&theme=light");
  await page.locator("html").evaluate((root) => {
    root.style.setProperty("--layout-card-padding", "28px");
    root.style.setProperty("--layout-row-padding", "18px");
  });
  const row = page.locator('[data-slot="record-list"] > article').first();
  await expect(row).toHaveCSS("padding-inline-start", "28px");
  await expect(row).toHaveCSS("padding-block-start", "18px");
  await open(page, "fixture=workspace&state=settings&pane=plugins&theme=light");
  await page.getByRole("button", { name: "Configure inputs", exact: true }).first().click();
  await page.locator("html").evaluate((root) => {
    root.style.setProperty("--layout-card-padding", "28px");
    root.style.setProperty("--layout-row-padding", "18px");
    root.style.setProperty("--layout-compact-gutter", "24px");
  });
  const modal = page.getByRole("dialog", { name: "Configure inputs", exact: true });
  await expect(modal.locator("header")).toHaveCSS("padding-inline-start", "28px");
  await expect(modal.locator('[data-slot="dialog-body"]')).toHaveCSS(
    "padding-inline-start",
    "28px",
  );
  await expect(modal.locator("footer")).toHaveCSS("padding-inline-start", "28px");
  await expect(modal.locator("footer")).toHaveCSS("padding-block-start", "18px");
  await page.setViewportSize({ width: 320, height: 568 });
  const box = (await modal.boundingBox())!;
  expect(box.x).toBe(24);
  expect(box.x + box.width).toBe(296);
});
