import { expect, test } from "./test";

test("overlapping real disclosures restore their shared viewport styles", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await page.goto("/visual/?fixture=agent&theme=light&state=tool-shells&motion=full");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("button", { name: /steps/ }).first().click();
  const rows = page.locator(
    "[data-slot='agent-activity-disclosure']:is([data-tool='shell'], [data-tool='apply_patch'])",
  );
  expect(await rows.count()).toBeGreaterThanOrEqual(2);
  for (const row of [rows.nth(0), rows.nth(1)]) {
    const trigger = row.locator("button[aria-expanded]").first();
    if ((await trigger.getAttribute("aria-expanded")) === "false") await trigger.click();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
  }
  await expect
    .poll(() =>
      rows.evaluateAll(
        (elements) =>
          elements
            .slice(0, 2)
            .flatMap((element) => element.getAnimations({ subtree: true }))
            .filter((animation) => animation.playState === "running").length,
      ),
    )
    .toBe(0);
  const viewport = page.locator(".msg-scroll-viewport");
  const original = await viewport.evaluate((element) => ({
    scrollbarWidth: (element as HTMLElement).style.scrollbarWidth,
    paddingLeft: (element as HTMLElement).style.paddingLeft,
    paddingRight: (element as HTMLElement).style.paddingRight,
  }));
  const locked = await rows.evaluateAll(async (elements) => {
    const selected = elements.slice(0, 2);
    const containers = selected.map((element) => {
      let ancestor = element.parentElement;
      while (ancestor) {
        if (["scroll", "auto"].includes(getComputedStyle(ancestor).overflowY)) return ancestor;
        ancestor = ancestor.parentElement;
      }
      return null;
    });
    for (const element of selected)
      element.querySelector<HTMLButtonElement>("button[aria-expanded]")!.click();
    await new Promise(requestAnimationFrame);
    return {
      shared: containers[0] !== null && containers[0] === containers[1],
      width: containers[0]?.style.scrollbarWidth,
      closingAnimations: selected
        .flatMap((element) => element.getAnimations({ subtree: true }))
        .filter(
          (animation) =>
            animation instanceof CSSTransition &&
            animation.transitionProperty === "grid-template-rows",
        ).length,
    };
  });
  expect(locked).toEqual({ shared: true, width: "none", closingAnimations: 2 });
  await expect
    .poll(() =>
      viewport.evaluate((element) => ({
        scrollbarWidth: (element as HTMLElement).style.scrollbarWidth,
        paddingLeft: (element as HTMLElement).style.paddingLeft,
        paddingRight: (element as HTMLElement).style.paddingRight,
      })),
    )
    .toEqual(original);
});
