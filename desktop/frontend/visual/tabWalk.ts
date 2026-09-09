import type { Page } from "@playwright/test";

/** Only a way out if the cycle never closes; the walk normally ends when the order does. */
const MAX_PRESSES = 200;

/**
 * Visit every stop in a route's tab order, once each.
 *
 * Two audits need this and neither should restate it, because each of the three subtleties cost
 * a round to find and each one fails by covering LESS while still reporting success.
 *
 * `activeElement === document.body` after a load is not the top of the order. Chromium's
 * sequential-focus navigation starting point is separate state, and from it the dock route
 * visits the LAST two controls, leaves the document, and only then enters at the first — so a
 * walk that trusted it compared two controls either side of that boundary and read a 984px
 * leftward jump. The rewind therefore presses Tab until leaving the document is what a press
 * produced; the next press enters at the true first stop.
 *
 * Identity is the element, marked in the page. A key built from tag, class prefix and text
 * ended one walk after a single stop, two controls sharing it, and collapsed the goal bar's
 * three icon buttons — whose labels are all empty — into one stop, which is exactly the group a
 * reordering shows up in.
 *
 * And the walk runs to closure rather than a fixed count. It was forty-five presses, a number
 * chosen without measuring: the cycles close at 24, 50, 22, 56 and 40, so two routes were being
 * audited as a prefix and the second pane resizer sat three stops past where one walk stopped.
 */
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
    // A roving group re-entered: not a new position, and not the end either.
    if (state === "repeat") continue;
    stops += 1;
    await visit();
  }
  return stops;
}
