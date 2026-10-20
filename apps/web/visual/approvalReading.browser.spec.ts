import { expect, test } from "./test";
for (const plan of [
  { theme: "light", font: 14, touch: false },
  { theme: "dark", font: 18, touch: false },
  { theme: "light", font: 18, touch: true },
]) {
  test.describe(`joined approval actions in ${plan.theme} at ${plan.font}px with ${plan.touch ? "touch" : "mouse"}`, () => {
    test.use({ hasTouch: plan.touch, reducedMotion: "no-preference" });

    test("keep their common height and seam while pressed", async ({ page }) => {
      await page.setViewportSize({ width: 1120, height: 720 });
      await page.goto(
        `/visual/?fixture=agent&state=waiting&theme=${plan.theme}&font-size=${plan.font}`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
      await page.evaluate(() => document.fonts.ready);
      const group = page.getByRole("group", { name: "Approval options", exact: true });
      const allow = group.getByRole("button", { name: "Allow once", exact: true });
      const options = group.getByRole("button", { name: "Approval options", exact: true });
      await allow.hover();
      const main = (await allow.boundingBox())!;
      const arrow = (await options.boundingBox())!;
      expect(arrow.y).toBeCloseTo(main.y, 1);
      expect(arrow.height).toBeCloseTo(main.height, 1);
      expect(arrow.x).toBeLessThanOrEqual(main.x + main.width);
      if (plan.touch) {
        expect(main.height).toBeGreaterThanOrEqual(44);
        expect(arrow.width).toBeGreaterThanOrEqual(44);
      }

      await page.mouse.down();
      try {
        await allow.evaluate(async (node) => {
          void getComputedStyle(node).scale;
          await Promise.allSettled(node.getAnimations().map((animation) => animation.finished));
        });
        const pressed = (await allow.boundingBox())!;
        for (const dimension of ["x", "y", "width", "height"] as const) {
          expect(pressed[dimension]).toBeCloseTo(main[dimension], 1);
        }
      } finally {
        await page.mouse.move(2, 2);
        await page.mouse.up();
      }
      await expect(allow).toBeVisible();
      await options.press("Enter");
      const menu = page.getByRole("menu");
      await expect(menu.getByRole("menuitem", { name: "Allow for this session" })).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(menu).toHaveCount(0);
      await expect(options).toBeFocused();
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
    });
  });
}
