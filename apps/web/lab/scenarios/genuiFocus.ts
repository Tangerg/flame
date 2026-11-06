import { expect, test, type Locator, type Page } from "./test";

// CSS frames around a native 200% window resize; this is browser geometry, not page zoom.
const WIDE_FRAME = { width: 756, height: 474 };
const NARROW_FRAME = { width: 720, height: 449 };
const SUBPIXEL_TOLERANCE = 0.5;

async function expectWithinHorizontalClip(
  page: Page,
  region: Locator,
  control: Locator,
  stage: string,
): Promise<void> {
  const scroller = await region.elementHandle();
  if (!scroller) throw new Error("The report table region is unavailable.");
  let geometry: unknown;
  try {
    await expect
      .poll(
        async () => {
          const measured = await control.evaluate((button, clip) => {
            const frame = clip.getBoundingClientRect();
            const left = frame.left + clip.clientLeft;
            const right = left + clip.clientWidth;
            const bounds = button.getBoundingClientRect();
            return {
              viewport: { width: innerWidth, height: innerHeight },
              clip: {
                left,
                right,
                clientLeft: clip.clientLeft,
                clientWidth: clip.clientWidth,
                scrollLeft: clip.scrollLeft,
                scrollWidth: clip.scrollWidth,
              },
              control: {
                left: bounds.left,
                right: bounds.right,
                top: bounds.top,
                bottom: bounds.bottom,
              },
              focused: document.activeElement === button,
              hiddenLeft: left - bounds.left,
              hiddenRight: bounds.right - right,
            };
          }, scroller);
          geometry = measured;
          return Math.max(measured.hiddenLeft, measured.hiddenRight);
        },
        { message: `${stage}: Sort by Review decision is clipped by its table region` },
      )
      .toBeLessThanOrEqual(SUBPIXEL_TOLERANCE);
  } finally {
    await test.info().attach(`final-sort-${stage}-geometry`, {
      body: JSON.stringify(geometry, null, 2),
      contentType: "application/json",
    });
    await test.info().attach(`final-sort-${stage}-frame`, {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
  }
}

async function openFinalSort(page: Page, theme: "light" | "dark") {
  await page.setViewportSize(WIDE_FRAME);
  await page.goto(
    `/lab/fixture.html?fixture=agent&state=genui-report&locale=en&theme=${theme}&font-size=18&sidebar=collapsed`,
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const draft = "Keep this draft while the report window narrows.";
  const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
  await composer.fill(draft);
  await page.locator('[data-tool="genui_ui_present"]').click();
  const report = page.getByRole("region", { name: "Agent plugin delivery review", exact: true });
  await expect(report).toBeVisible();

  const tableRegion = report.getByRole("region", {
    name: "Plugin delivery responsibilities",
    exact: true,
  });
  const table = tableRegion.getByRole("table", { name: "Plugin delivery responsibilities" });
  const generated = table.getByRole("checkbox", { name: "Select generated", exact: true });
  const selected = report.getByText("Selected evidence rows", { exact: true }).locator("..");
  await generated.click();
  await expect(generated).toBeChecked();
  await expect(selected).toContainText("1");

  const search = report.getByRole("searchbox", {
    name: "Search Plugin delivery responsibilities",
    exact: true,
  });
  await search.fill("no responsibility matches this query");
  await expect(table.getByRole("row")).toHaveCount(2);
  await expect(generated).toHaveCount(0);
  await report.getByRole("button", { name: "Clear search", exact: true }).click();
  await expect(search).toBeFocused();
  await expect(search).toHaveValue("");
  await expect(generated).toBeChecked();
  await expect(selected).toContainText("1");

  const sortButtons = table.getByRole("button", { name: /^Sort by / });
  await expect(sortButtons).toHaveCount(8);
  await page.keyboard.press("Tab");
  await expect(tableRegion).toBeFocused();
  await page.keyboard.press("Tab");
  const capability = table.getByRole("button", { name: "Sort by Capability", exact: true });
  await expect(capability).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(table.getByRole("columnheader", { name: /Capability/ })).toHaveAttribute(
    "aria-sort",
    "ascending",
  );
  await expect(capability).toBeFocused();
  for (let index = 1; index < 8; index += 1) {
    await page.keyboard.press("Tab");
    await expect(sortButtons.nth(index)).toBeFocused();
  }
  const reviewDecision = table.getByRole("button", {
    name: "Sort by Review decision",
    exact: true,
  });
  await expect(reviewDecision).toBeFocused();
  await expectWithinHorizontalClip(page, tableRegion, reviewDecision, `${theme}-initial`);

  return { draft, composer, report, tableRegion, generated, selected, search, reviewDecision };
}

export async function keepFinalSortRevealed(page: Page, theme: "light" | "dark"): Promise<void> {
  const { draft, composer, report, tableRegion, generated, selected, search, reviewDecision } =
    await openFinalSort(page, theme);

  for (const [stage, frame] of [
    ["narrowed", NARROW_FRAME],
    ["expanded", WIDE_FRAME],
  ] as const) {
    await page.setViewportSize(frame);
    await expect(reviewDecision).toBeFocused();
    await expectWithinHorizontalClip(page, tableRegion, reviewDecision, `${theme}-${stage}`);
    await expect(selected).toContainText("1");
    await expect(composer).toHaveValue(draft);
  }

  await search.fill("Generated report");
  await expect(generated).toBeChecked();
  await report.getByRole("button", { name: "Clear search", exact: true }).click();
  await expect(search).toBeFocused();
  await composer.click();
  await expect(composer).toBeFocused();
  await expect(composer).toHaveValue(draft);
}

export async function respectHorizontalReading(page: Page, theme: "light" | "dark"): Promise<void> {
  const { draft, composer, tableRegion, reviewDecision } = await openFinalSort(page, theme);
  const original = await tableRegion.evaluate((region) => region.scrollLeft);
  await tableRegion.hover({ position: { x: 24, y: 64 } });
  await page.mouse.wheel(-180, 0);
  await expect
    .poll(() => tableRegion.evaluate((region) => region.scrollLeft))
    .toBeLessThan(original - 100);
  const manual = await tableRegion.evaluate((region) => region.scrollLeft);
  await expect(reviewDecision).toBeFocused();
  for (const frame of [NARROW_FRAME, WIDE_FRAME]) {
    await page.setViewportSize(frame);
    await expect(reviewDecision).toBeFocused();
    await expect
      .poll(() => tableRegion.evaluate((region) => region.scrollLeft))
      .toBeCloseTo(manual, 0);
  }
  await test.info().attach(`${theme}-manual-horizontal-reading`, {
    body: await page.screenshot({ animations: "disabled" }),
    contentType: "image/png",
  });

  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(reviewDecision).toBeFocused();
  await expectWithinHorizontalClip(page, tableRegion, reviewDecision, `${theme}-keyboard-return`);

  await composer.click();
  await expect(composer).toBeFocused();
  await tableRegion.hover({ position: { x: 24, y: 64 } });
  await page.mouse.wheel(-180, 0);
  await expect
    .poll(() => tableRegion.evaluate((region) => region.scrollLeft))
    .toBeLessThan(original - 100);
  const outside = await tableRegion.evaluate((region) => region.scrollLeft);
  for (const frame of [NARROW_FRAME, WIDE_FRAME]) {
    await page.setViewportSize(frame);
    await expect(composer).toBeFocused();
    await expect
      .poll(() => tableRegion.evaluate((region) => region.scrollLeft))
      .toBeCloseTo(outside, 0);
    await expect(composer).toHaveValue(draft);
  }
}
