import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const browserName of ["chromium", "webkit"] as const) {
  test.extend({ browserName })(
    `${browserName} reasoning follows growth and preserves a paused reader through closing`,
    async ({ page }) => {
      await page.goto("/lab/fixture.html?fixture=agent&state=waves&theme=light&motion=off");
      await expectLabReady(page);
      const disclosure = page
        .locator('[data-slot="agent-activity-disclosure"]')
        .filter({ hasText: "Thinking" });
      const trigger = disclosure.locator("button[aria-expanded]").first();
      await trigger.click();
      const reader = disclosure.locator('[data-slot="reasoning-scroller"]');
      const grow = (height: number) =>
        reader.evaluate((node, height) => {
          (node.firstElementChild as HTMLElement).style.minHeight = `${height}px`;
        }, height);
      const distance = () =>
        reader.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop);
      await grow(1800);
      await expect.poll(distance).toBeLessThanOrEqual(2);
      await reader.evaluate(async (node) => {
        node.scrollTop = 0;
        node.dispatchEvent(new Event("scroll"));
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        );
      });
      await expect.poll(() => reader.evaluate((node) => node.scrollTop)).toBe(0);
      await grow(2200);
      await expect.poll(distance).toBeGreaterThan(1500);
      await expect(reader).toHaveJSProperty("scrollTop", 0);
      await trigger.click();
      await expect(reader).toBeHidden();
      await trigger.click();
      await expect(reader).toBeVisible();
      await expect(reader).toHaveJSProperty("scrollTop", 0);
      await reader.evaluate(async (node) => {
        node.scrollTop = node.scrollHeight;
        node.dispatchEvent(new Event("scroll"));
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
        );
      });
      await expect.poll(distance).toBeLessThanOrEqual(2);
      await grow(2600);
      await expect.poll(distance).toBeLessThanOrEqual(2);
      await expect(trigger).toBeFocused();
    },
  );
}
