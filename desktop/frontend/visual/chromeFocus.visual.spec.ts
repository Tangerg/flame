import { expect, test } from "./test";
import { eachTabStop } from "./tabWalk";

// `data-chrome-focus` turns the focus ring off on a promise: that a row state shows focus
// instead. `menu.tsx` states it — a popup takes focus so the keyboard can drive it, and the
// highlighted ITEM is the indicator. Nothing checked the promise was kept, and three controls
// were not keeping it: the question card was a tab stop that showed nothing, the header diff
// stat had no row around it at all, and the ACTIVE dock tab's stand-in was
// `focus-within:text-fg`, which it already had.
//
// Measured the way a person meets it: press Tab until a control that opted out has focus,
// photograph it, blur, photograph again. Identical bytes mean a keyboard user sees nothing.
//
// Real Tab, not `element.focus()`. Programmatic focus does not run the roving-tabindex
// activation a dock tab uses, so it reported six controls as silent that a keyboard shows
// plainly — and it lands on the first tabbable before the walk starts, which reads as "no
// change" for whatever that happens to be. Focus IS restored programmatically after each
// photograph, which is a different thing: the visual state has already been captured, and all
// that is needed back is the position for the next press.

const ROUTES = [
  "fixture=agent&state=question",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

const SETTLE_MS = 500;
/** Two full passes of a route's order, from the cold figure — the dev server is fresh per run. */
const ROUTE_BUDGET_MS = 120_000;

// The walk itself is `tabWalk.ts`, shared with the two ring audits. This file used to press
// Tab forty times per route and, after each finding, blur and count its way back with
// step-plus-one presses — quadratic, and short: the dock route's order does not close until
// fifty. Both of those made it cover less while reporting the same.

test("a control that turns off the ring shows focus some other way", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const silent: string[] = [];
  let optOuts = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");

    await eachTabStop(page, async () => {
      const meta = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement;
        if (!active.hasAttribute("data-chrome-focus")) return null;
        if (!active.matches(":focus-visible")) return null;
        const box = active.getBoundingClientRect();
        // The comparison is two photographs of one region, so the region has to be on screen
        // and whole in both of them.
        if (box.width === 0 || box.y < 0 || box.y + box.height > 720) return null;
        if (box.x + box.width > 1120) return null;
        // Marked so focus can be returned to this exact element, rather than counted back to.
        active.dataset.chromeFocusProbe = "";
        return {
          label: (
            active.getAttribute("aria-label") ??
            active.getAttribute("title") ??
            active.textContent ??
            ""
          )
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: active.tagName.toLowerCase(),
          clip: {
            x: Math.max(0, box.x - 3),
            y: Math.max(0, box.y - 3),
            width: box.width + 6,
            height: box.height + 6,
          },
        };
      });
      if (!meta) return;
      optOuts += 1;

      const focused = await page.screenshot({ clip: meta.clip });
      await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
      await page.waitForTimeout(SETTLE_MS);
      const blurred = await page.screenshot({ clip: meta.clip });
      if (Buffer.compare(focused, blurred) === 0) {
        silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
      }

      // Focus goes back to the element it was on, so the walk carries on from where it was.
      // It used to blur and then press Tab step-plus-one times to count its way back, which
      // made the walk quadratic and assumed a press from nowhere lands at the top of the order
      // — and it does not.
      await page.evaluate(() => {
        const probe = document.querySelector<HTMLElement>("[data-chrome-focus-probe]");
        probe?.focus();
        probe?.removeAttribute("data-chrome-focus-probe");
      });
    });
  }

  // A walk that met no opt-out holds no promise and reports nothing.
  expect(optOuts, "the walk has to reach controls that opted out").toBeGreaterThan(8);
  expect(
    [...new Set(silent)],
    "controls with `data-chrome-focus` that show nothing when a keyboard reaches them",
  ).toEqual([]);
});
