import { expect, test } from "./test";

// Escape closes the thing that is open, once, and focus comes back to what opened it.
//
// That is the whole contract a dialog has with the keyboard, and Base UI implements it — so the
// only way to lose it is for the product to put something else on the dismiss stack. It did: a
// lightbox focused the first tabbable control in it, which in every case is an icon button with
// a tip, so opening one moved focus to a control the person never navigated to, its tooltip
// opened, and the tooltip sat above the dialog and took the first Escape. Measured on the image
// gallery: focus landed on "Download image", Escape #1 closed that tip and left the dialog up,
// Escape #2 closed the dialog. Pressing Escape once appeared to do nothing.
//
// Per-trigger reload, and that is not a detail. The first version of this swept the triggers on
// one page load and reported nine failures; one was real and the other eight were the popup left
// open by the previous iteration, which makes "did not close" true for everything after it. A
// sweep that mutates the page it is measuring has to start over each time.
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

      // Focus has to land, or pressing a key proves nothing about this control — the lesson
      // `activationFocus.visual.spec.ts` is built on.
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
      // Closing without handing focus back leaves the keyboard on `<body>`, which is the defect
      // three other audits in this suite exist for.
      const returned = await trigger.evaluate((node) => document.activeElement === node);
      if (!returned) broken.push(`${route} "${name}" — closed, but focus did not come back`);
    }
  }

  // Floor, not a target: a sweep that opened nothing agrees with any product.
  expect(opened, "the sweep has to actually open popups by keyboard").toBeGreaterThan(10);
  expect([...new Set(broken)], "controls that broke the dismiss contract").toEqual([]);
});
