import { expect, test } from "./test";

const ROUTES = [
  "fixture=agent&state=long-content",
  "fixture=agent&state=narrative",
  "fixture=workspace&state=settings&pane=appearance",
  "fixture=shell&state=populated",
] as const;

const POPUP = '[role="dialog"],[role="menu"],[role="listbox"]';

test("one Escape closes what a control opened, and focus comes back", async ({ page }) => {
  test.setTimeout(ROUTES.length * 90_000 + 30_000);
  await page.setViewportSize({ width: 1472, height: 900 });

  const broken: string[] = [];
  let opened = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(300);
    const total = await page.locator("[aria-haspopup]:not([data-fixture-chrome] *)").count();

    for (let index = 0; index < Math.min(total, 12); index += 1) {
      await page.goto(`/visual/?${route}&theme=light`);
      await page.locator("html[data-visual-ready]").waitFor();
      await page.waitForTimeout(250);

      const trigger = page.locator("[aria-haspopup]:not([data-fixture-chrome] *)").nth(index);
      if ((await trigger.count()) === 0) continue;
      const name = await trigger.evaluate(
        (node) =>
          `${node.getAttribute("aria-label") ?? (node.textContent ?? "").trim().slice(0, 18)}`,
      );
      const before = await page.locator(POPUP).count();

      const landed = await trigger.evaluate((node) => {
        (node as HTMLElement).focus();
        return document.activeElement === node;
      });
      if (!landed) continue;

      await page.keyboard.press("Enter");
      await page.waitForTimeout(300);
      if ((await page.locator(POPUP).count()) <= before) continue;
      opened += 1;

      await page.keyboard.press("Escape");
      await page.waitForTimeout(320);

      if ((await page.locator(POPUP).count()) > before) {
        broken.push(`${route} "${name}" — one Escape did not close it`);
        continue;
      }
      const returned = await trigger.evaluate((node) => document.activeElement === node);
      if (!returned) broken.push(`${route} "${name}" — closed, but focus did not come back`);
    }
  }

  expect(opened, "the sweep has to actually open popups by keyboard").toBeGreaterThan(10);
  expect([...new Set(broken)], "controls that broke the dismiss contract").toEqual([]);
});
