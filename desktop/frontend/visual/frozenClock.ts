import { expect, type Page } from "@playwright/test";
import { VISUAL_NOW } from "./agentFixtureFacts";

const ELAPSED_TICK_MS = 1000;
const WORKING_LINE = '[data-slot="agent-working"]';
const FIRST_TURN = "[data-turn-id]";

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
