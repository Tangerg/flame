import { expect, test } from "./test";
test("expanded tool groups keep every call reachable while the Dock resizes", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-files&agent-state=tool-search&theme=light&font-size=18&locale=en",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /calls/ }).first().click();
  const glob = page.locator('[data-tool="glob"]');
  await glob.click();
  await expect(page.locator('[data-slot="recorded-result-viewport"]')).toContainText(
    "runtime/internal/session/store_test.go",
  );
  const resizer = page.getByRole("separator", { name: "Resize right workspace" });
  for (const key of ["End", "Home"]) {
    await resizer.press(key);
    for (const name of ["glob", "search_memory", "search_tools"]) {
      const row = page.locator(`[data-tool="${name}"]`);
      await row.scrollIntoViewIfNeeded();
      expect(
        await row.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          const frame = element.closest('[data-slot="agent-activity-disclosure"]')!;
          const outer = frame.getBoundingClientRect();
          const hit = document.elementFromPoint(
            bounds.x + bounds.width / 2,
            bounds.y + bounds.height / 2,
          );
          return {
            contained: bounds.bottom <= outer.bottom + 1,
            hit: hit !== null && element.contains(hit),
          };
        }),
      ).toEqual({ contained: true, hit: true });
    }
  }
});

test("a tool group can reverse closing before its content disappears", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto(
    "/visual/?fixture=agent&state=tool-search&theme=dark&font-size=18&locale=en&motion=full",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  const trigger = page.getByRole("button", { name: /calls/ }).first();
  const frame = page.locator('[data-slot="agent-activity-disclosure"]').filter({ has: trigger });
  await trigger.click();
  await frame.evaluate(async (element) => {
    await Promise.all(
      element.getAnimations({ subtree: true }).map((animation) => animation.finished),
    );
  });
  const expandedHeight = (await frame.boundingBox())!.height;
  await trigger.click();
  const midway = await frame.evaluate((element) => {
    const closing = element
      .getAnimations({ subtree: true })
      .find(
        (animation) =>
          animation instanceof CSSTransition &&
          animation.transitionProperty === "grid-template-rows",
      );
    if (!closing?.effect) throw new Error("The group has no reversible height transition");
    const duration = closing.effect.getTiming().duration;
    if (typeof duration !== "number" || duration <= 0)
      throw new Error("Full motion requires a nonzero transition duration");
    closing.pause();
    closing.currentTime = duration / 2;
    const height = element.getBoundingClientRect().height;
    const headerHeight = element
      .querySelector('[data-slot="agent-activity-header"]')!
      .getBoundingClientRect().height;
    (element.querySelector("button[aria-controls]") as HTMLButtonElement).click();
    return { height, headerHeight };
  });
  expect(midway.height).toBeGreaterThan(midway.headerHeight + 1);
  expect(midway.height).toBeLessThan(expandedHeight - 1);
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await frame.evaluate(async (element) => {
    await Promise.all(
      element.getAnimations({ subtree: true }).map((animation) => animation.finished),
    );
  });
  expect(Math.abs((await frame.boundingBox())!.height - expandedHeight)).toBeLessThanOrEqual(1);
  await expect(page.locator('[data-tool="search_tools"]')).toBeVisible();
});
