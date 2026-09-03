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
 * Then the transcript's own origin has to stop moving, to the fraction. The scroll settle
 * earlier compares an integer `scrollTop`, which is blind to the sub-pixel the block still
 * has to give: the delegated golden landed a pixel apart between runs, identical in content,
 * and every glyph in the frame differed because of it.
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
