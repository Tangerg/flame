import { expect, test, type Locator, type Page } from "./test";
import { expectLabReady } from "../readiness";
async function open(page: Page, query: string) {
  await page.goto(`/lab/fixture.html?${query}&font-size=14`);
  await expectLabReady(page);
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
      page.getByRole("button", { name: "gpt-5 · high, Model", exact: true }),
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
    root.style.setProperty("--floating-panel-radius", "28px");
  });
  const editor = page.getByRole("dialog", { name: "New schedule", exact: true });
  await expect(editor.locator('[data-slot="dialog-body"]')).toHaveCSS(
    "padding-inline-start",
    "32px",
  );
  await expect(editor).toHaveCSS("border-top-left-radius", "28px");
  const group = editor.getByRole("group", { name: "Timing and workspace", exact: true });
  await expect(group.locator(":scope > div")).toHaveCSS("row-gap", "24px");
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(editor).toHaveCount(0);
  const panel = page
    .getByRole("region", { name: "Schedules", exact: true, includeHidden: true })
    .and(page.locator('[data-slot="panel"]'));
  const pageEdge = (await panel.boundingBox())!.x;
  expect((await panel.locator('[data-slot="panel-controls"]').boundingBox())!.x - pageEdge).toBe(
    40,
  );
});

test("segments slide continuously and reverse from their painted position", async ({
  page,
}, testInfo) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await open(page, "fixture=atoms&theme=light&motion=full");
  const tabs = page.getByRole("radiogroup", { name: "Diagnostics", exact: true });
  await tabs.scrollIntoViewIfNeeded();
  const trace = tabs.getByRole("radio", { name: "Trace", exact: true });
  const metrics = tabs.getByRole("radio", { name: "Metrics", exact: true });
  const logs = tabs.getByRole("radio", { name: "Logs", exact: true });
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
  await expect(trace).toHaveAttribute("aria-checked", "true");
  await expect(tabs.locator('[data-slot="item-highlight"]')).toHaveCount(1);

  await page.setViewportSize({ width: 720, height: 900 });
  await tabs.scrollIntoViewIfNeeded();
  const resizedReading = tabs.evaluate(async (track) => {
    const frames = [];
    const start = performance.now();
    while (performance.now() - start < 420) {
      const indicator = track.querySelector('[data-slot="item-highlight"]')!;
      frames.push({
        track: track.getBoundingClientRect().toJSON(),
        indicator: indicator.getBoundingClientRect().toJSON(),
      });
      await new Promise(requestAnimationFrame);
    }
    return frames;
  });
  await metrics.click();
  const resizedFrames = await resizedReading;
  await testInfo.attach("segment-after-resize-frames", {
    body: JSON.stringify(resizedFrames),
    contentType: "application/json",
  });
  expect(resizedFrames.length).toBeGreaterThan(1);
  for (const frame of resizedFrames) {
    expect(frame.indicator.left, JSON.stringify(frame)).toBeGreaterThanOrEqual(
      frame.track.left - 1,
    );
    expect(frame.indicator.top, JSON.stringify(frame)).toBeGreaterThanOrEqual(frame.track.top - 1);
    expect(frame.indicator.right, JSON.stringify(frame)).toBeLessThanOrEqual(frame.track.right + 1);
    expect(frame.indicator.bottom, JSON.stringify(frame)).toBeLessThanOrEqual(
      frame.track.bottom + 1,
    );
  }
  const destination = (await metrics.boundingBox())!;
  const last = resizedFrames.at(-1)!.indicator;
  expect(Math.abs(last.x - destination.x)).toBeLessThan(1);
  expect(Math.abs(last.y - destination.y)).toBeLessThan(1);
  expect(Math.abs(last.width - destination.width)).toBeLessThan(1);
  expect(Math.abs(last.height - destination.height)).toBeLessThan(1);
  await expect(metrics).toHaveAttribute("aria-checked", "true");
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
    .getByRole("radiogroup", { name: "Diagnostics", exact: true })
    .getByRole("radio", { name: "Metrics", exact: true });
  await metrics.click();
  const indicator = metrics.locator('[data-slot="item-highlight"]');
  const reading = indicator.evaluate(async (element) => {
    const tab = element.closest('[role="radio"]')!;
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

for (const [engine, engineTest] of [
  ["chromium", test],
  ["webkit", test.extend({ browserName: "webkit" })],
] as const) {
  engineTest.describe(`Skills selection reading in ${engine}`, () => {
    for (const preference of ["reduce", "no-preference"] as const) {
      for (const appMotion of ["off", "full"] as const) {
        engineTest(
          `the unchanged segment stays behind its label through proposal reflow and wheel reading app ${appMotion} system ${preference}`,
          async ({ page }, testInfo) => {
            await page.emulateMedia({ reducedMotion: preference });
            await page.setViewportSize({ width: 1440, height: 900 });
            await page.goto(
              `/lab/fixture.html?fixture=workspace&state=dock-skills&locale=zh&theme=dark&font-size=18&motion=${appMotion}&skills-commands=accept`,
            );
            await expectLabReady(page);
            await page
              .getByRole("button", { name: "Collapse the workspace fixture sidebar" })
              .click();
            const panel = page
              .getByRole("region", { name: "技能", exact: true })
              .and(page.locator('[data-slot="panel"]'));
            await panel.getByRole("radio", { name: "技能提议", exact: true }).click();
            for (let step = 1; step <= 20; step += 1) {
              const scale = 1 + step / 20;
              await page.setViewportSize({
                width: Math.round(1440 / scale),
                height: Math.round(900 / scale),
              });
              await page.evaluate(async () => {
                await new Promise(requestAnimationFrame);
                await new Promise(requestAnimationFrame);
              });
            }
            await page.getByRole("button", { name: "全宽打开材料", exact: true }).click();
            await panel
              .getByRole("group", { name: "risk-first-review 的提议操作", exact: true })
              .getByRole("button", { name: "拒绝", exact: true })
              .click();
            const review = panel.getByRole("region", { name: "确认提议决定", exact: true });
            const firstCancel = review.getByRole("button", { name: "取消", exact: true });
            await expect(firstCancel).toBeFocused();
            const firstCancelBounds = (await firstCancel.boundingBox())!;
            await page.mouse.move(
              firstCancelBounds.x + firstCancelBounds.width / 2,
              firstCancelBounds.y + firstCancelBounds.height / 2,
            );
            await page.mouse.wheel(0, -600);
            for (const size of [
              { width: 756, height: 475 },
              { width: 720, height: 450 },
            ]) {
              await page.setViewportSize(size);
              await page.evaluate(async () => {
                for (let frame = 0; frame < 8; frame += 1) {
                  await new Promise(requestAnimationFrame);
                }
              });
            }
            await page.keyboard.press("Shift+Tab");
            await page.keyboard.press("Enter");
            await page.keyboard.press("Tab");
            await page.keyboard.press("Tab");
            await page.keyboard.press("Enter");
            await expect(
              panel.getByRole("searchbox", { name: "搜索此目录", exact: true }),
            ).toBeFocused();
            await expect(panel).toContainText("1 项待处理提议");
            for (let step = 0; step < 4; step += 1) await page.keyboard.press("Tab");
            await expect(
              panel
                .getByRole("group", { name: "release-notes 的提议操作", exact: true })
                .getByRole("button", { name: "批准", exact: true }),
            ).toBeFocused();
            const indicator = panel
              .getByRole("radio", { name: "技能提议", exact: true })
              .locator('[data-slot="item-highlight"]');
            const reading = indicator.evaluate(async (element) => {
              const radio = element.closest('[role="radio"]')!;
              const frames = [];
              const start = performance.now();
              while (performance.now() - start < 840) {
                const radioTransform = getComputedStyle(radio).transform;
                frames.push({
                  radio: radio.getBoundingClientRect().toJSON(),
                  indicator: element.getBoundingClientRect().toJSON(),
                  radioTransform,
                  radioUntransformed:
                    radioTransform === "none" || new DOMMatrix(radioTransform).isIdentity,
                  transform: getComputedStyle(element).transform,
                });
                await new Promise(requestAnimationFrame);
              }
              return frames;
            });
            await page.keyboard.press("Enter");
            const cancel = review.getByRole("button", { name: "取消", exact: true });
            await expect(cancel).toBeFocused();
            const cancelBounds = (await cancel.boundingBox())!;
            await page.mouse.move(
              cancelBounds.x + cancelBounds.width / 2,
              cancelBounds.y + cancelBounds.height / 2,
            );
            await page.mouse.wheel(0, -600);
            const frames = await reading;
            await testInfo.attach("segment-reading-frames", {
              body: JSON.stringify(frames),
              contentType: "application/json",
            });
            await testInfo.attach(`segment-reading-${engine}-${appMotion}`, {
              body: await page.screenshot({ animations: "disabled" }),
              contentType: "image/png",
            });
            for (const frame of frames) {
              expect(frame.radioUntransformed, JSON.stringify(frame)).toBe(true);
              if (preference === "reduce" || appMotion === "off") {
                expect(frame.transform, JSON.stringify(frame)).toBe("none");
              }
              for (const edge of ["left", "top", "right", "bottom"] as const) {
                expect(
                  Math.abs(frame.indicator[edge] - frame.radio[edge]),
                  JSON.stringify(frame),
                ).toBeLessThan(1);
              }
            }
          },
        );
      }
    }
  });
}

test("multiline labels keep neighboring fields aligned at their control size", async ({ page }) => {
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
  const labelTextHeight = (label: Locator) =>
    label.evaluate((node) => {
      const range = document.createRange();
      range.selectNodeContents(node);
      return range.getBoundingClientRect().height;
    });
  const shortLabel = form.getByText("Short label", { exact: true });
  const longLabel = form.getByText(
    "A longer translated field label that wraps onto several lines",
    { exact: true },
  );
  expect(await labelTextHeight(longLabel)).toBeGreaterThan(await labelTextHeight(shortLabel));
  expect(longBox.x).toBeGreaterThan(shortBox.x);
  expect(longBox.y).toBeCloseTo(shortBox.y, 1);
  expect(shortBox.height).toBe(32);
  expect(longBox.height).toBe(32);
});

test("fields share their text inset and reset native appearance", async ({ page }) => {
  await open(page, "fixture=atoms&theme=light");
  const input = page.getByRole("textbox", { name: "Empty", exact: true });
  const area = page.getByRole("textbox", { name: "Note", exact: true });
  const picker = page.getByRole("button", { name: "gpt-5 · high, Model", exact: true });
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
  const modal = page.getByRole("dialog", { name: /^Configure inputs: / });
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
