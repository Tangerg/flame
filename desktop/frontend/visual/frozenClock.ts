import { expect, type Page } from "@playwright/test";
import { VISUAL_NOW } from "./agentFixtureFacts";

const ELAPSED_TICK_MS = 1000;
const WORKING_LINE = '[data-slot="agent-working"]';
const FIRST_TURN = "[data-turn-id]";

/**
 * Stop the clock at the instant the fixtures are written for, NOT at whatever the harness
 * clock has drifted to: the fixture keeps `Date.now` advancing through production bootstrap
 * so use-stick-to-bottom can complete its frame waits, and it then advances by the page's
 * real age — the one thing about a frame that load changes.
 *
 * The wait afterwards has to outlast the interval the elapsed label re-reads on. A shorter
 * one returns while the label still holds its pre-freeze value, which is how `390m 1s` and
 * `390m 2s` both reached goldens; only states that show the label pay for it.
 *
 * Then every turn has to have been laid out once, and the frame's origin has to stop moving
 * to the FRACTION. Both come from the same place: a turn carries `content-visibility: auto`
 * with an `auto 220px` intrinsic size, so one the browser has never measured contributes
 * 220px and its real height afterwards — measured at 98px for a short user turn. Two layouts
 * of one transcript, which is how the delegated golden came to differ by exactly 9037 pixels
 * whenever it differed at all, with identical content one pixel apart. Scrolling each turn
 * through the viewport resolves them the way a reader would; overriding the property instead
 * is NOT layout-neutral and moved twenty-six goldens when it was tried.
 *
 * Production is unaffected and was measured before this was written: Chromium's scroll
 * anchoring holds the visible content while the sizes correct, so only the scrollbar's own
 * range moves.
 */
export async function freezeVisualClock(page: Page): Promise<void> {
  await page.evaluate((frozen) => {
    Date.now = () => frozen;
  }, VISUAL_NOW);

  const working = page.locator(WORKING_LINE);
  if ((await working.count()) > 0) {
    await expect
      .poll(async () => {
        const before = await working.innerText();
        await page.waitForTimeout(ELAPSED_TICK_MS + 100);
        return before === (await working.innerText());
      })
      .toBe(true);
  }

  await page.evaluate(async (selector) => {
    const origin = () => document.querySelector(selector)?.getBoundingClientRect().top;
    if (origin() === undefined) return;
    const frame = () => new Promise((resolve) => requestAnimationFrame(resolve));
    for (let stable = 0, previous = NaN; stable < 5;) {
      await frame();
      const current = origin() ?? NaN;
      stable = current === previous ? stable + 1 : 0;
      previous = current;
    }
  }, FIRST_TURN);
}
