import type { Page } from "@playwright/test";

const MAX_PRESSES = 200;

export async function eachTabStop(page: Page, visit: () => Promise<void>): Promise<number> {
  const atBody = () => page.evaluate(() => document.activeElement === document.body);

  for (let press = 0; press < MAX_PRESSES; press += 1) {
    await page.keyboard.press("Tab");
    await page.waitForTimeout(40);
    if (await atBody()) break;
  }

  let stops = 0;
  for (let step = 0; step < MAX_PRESSES; step += 1) {
    await page.keyboard.press("Tab");
    await page.waitForTimeout(60);
    const state = await page.evaluate(() => {
      const active = document.activeElement as HTMLElement | null;
      if (!active || active === document.body) return "done";
      if (active.dataset.tabWalkSeen !== undefined) return "repeat";
      active.dataset.tabWalkSeen = "";
      return "fresh";
    });
    if (state === "done") break;
    if (state === "repeat") continue;
    stops += 1;
    await visit();
  }
  return stops;
}
