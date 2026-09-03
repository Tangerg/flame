import { expect, type Page } from "@playwright/test";
import { VISUAL_NOW } from "./agentFixtureFacts";

const ELAPSED_TICK_MS = 1000;
const WORKING_LINE = '[data-slot="agent-working"]';

/**
 * Stop the clock at the instant the fixtures are written for, NOT at whatever the harness
 * clock has drifted to: the fixture keeps `Date.now` advancing through production bootstrap
 * so use-stick-to-bottom can complete its frame waits, and it then advances by the page's
 * real age — the one thing about a frame that load changes.
 *
 * The wait afterwards has to outlast the interval the elapsed label re-reads on. A shorter
 * one returns while the label still holds its pre-freeze value, which is how `390m 1s` and
 * `390m 2s` both reached goldens; only states that show the label pay for it.
 */
export async function freezeVisualClock(page: Page): Promise<void> {
  await page.evaluate((frozen) => {
    Date.now = () => frozen;
  }, VISUAL_NOW);

  const working = page.locator(WORKING_LINE);
  if ((await working.count()) === 0) return;
  await expect
    .poll(async () => {
      const before = await working.innerText();
      await page.waitForTimeout(ELAPSED_TICK_MS + 100);
      return before === (await working.innerText());
    })
    .toBe(true);
}
