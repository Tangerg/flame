import { expect, test } from "./test";
import { eachTabStop } from "./tabWalk";

// Where the keyboard goes, and in what order.
//
// Three audits already look at the focus ring — whether it has room to draw, whether it paints,
// whether the opt-outs keep their promise — and none of them asks about the PATH. A ring on the
// right control in the wrong order is still a keyboard user losing their place.
//
// Both invariants below held when this was written, and the point of writing them down is that
// the measurement was wrong twice before it said so. Comparing viewport coordinates makes every
// step down a scrolling list look like a jump back up, because focus scrolls its container —
// which reported the sidebar's rows and two settings rows as reordered when nothing was. And a
// full-height pane resizer reports the top of the page, which looks like a 676px jump backwards
// from the last control in the window.
//
// So position is read in the nearest scroller's CONTENT space, which focus scrolling does not
// move, consecutive stops are only compared when they share that scroller, and anything tall
// enough to span a band is not compared at all.
//
// Shift+Tab was measured too, and retraced the forward path exactly on all four routes. It is
// not asserted here: three attempts to break it through product code all failed, because Base
// UI owns the roving focus these groups use and the repository forbids hand-rolling it. An
// assertion nobody has seen fail is not a guard, and this one would have been guarding a
// promise the design system already keeps.

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
];

// The walk ends when focus returns to where it started, so it covers a route rather than
// sampling it. It was a flat 45 presses, which was a number chosen without measuring: the dock
// route closes its cycle at 50 and the narrative route at 56, so the goal bar's actions were
// never reached and the guard was quietly auditing a prefix. The cap is only a way out if the
// cycle never closes.

/** A control taller than this spans bands rather than sitting in one, so its top says nothing. */
const BAND_HEIGHT = 400;

// No pixel thresholds. A first attempt used 24px down and 40px left, and a row of three icon
// buttons reversed on screen while keeping its DOM order went undetected — the jump between
// adjacent 24px controls is about 32px, under the threshold meant to tolerate layout noise. The
// relation that does not need tuning is whether the boxes OVERLAP: a stop that is entirely
// above, or entirely to the left of, the one before it went backwards, and two stops that
// overlap did not.
const fullyAbove = (next: Box, previous: Box) => next.bottom <= previous.top;
const fullyLeft = (next: Box, previous: Box) => next.right <= previous.left;
/** One row, decided by whether the two boxes overlap vertically at all. Leftward only counts
 *  as backwards WITHIN a row: a new row starting further left is ordinary reading order, and
 *  a first attempt without this flagged every consecutive pair of settings rows. */
const sameRow = (next: Box, previous: Box) =>
  next.top < previous.bottom && previous.top < next.bottom;
/** Entirely above but further right is a column advance, which reading order allows. */
const advancedColumn = (next: Box, previous: Box) => next.left >= previous.right;

/** Two full passes of a route's order, each press settling, with room for a loaded machine. */
const ROUTE_BUDGET_MS = 120_000;

interface Box {
  top: number;
  bottom: number;
  left: number;
  right: number;
}

test("the keyboard walks in reading order", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const jumps: string[] = [];
  const stopsPerRoute: string[] = [];
  let stops = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(400);

    const walk: (Box & { label: string; height: number; scroller: string })[] = [];

    await eachTabStop(page, async () => {
      const at = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement;
        const box = active.getBoundingClientRect();

        // Position inside the nearest scrolling ancestor's content, which does not move when
        // focus scrolls that ancestor.
        let scroller = "root";
        let cy = box.y + window.scrollY;
        let cx = box.x + window.scrollX;
        for (let parent = active.parentElement; parent; parent = parent.parentElement) {
          const style = getComputedStyle(parent);
          const scrolls =
            style.overflowY === "auto" ||
            style.overflowY === "scroll" ||
            style.overflowX === "auto" ||
            style.overflowX === "scroll";
          if (!scrolls) continue;
          const rect = parent.getBoundingClientRect();
          parent.dataset.keyboardScroller ??= String(nextScrollerId());
          scroller = parent.dataset.keyboardScroller;
          cy = box.y - rect.top + parent.scrollTop;
          cx = box.x - rect.left + parent.scrollLeft;
          break;
        }
        return {
          label: (
            active.getAttribute("aria-label") ??
            active.getAttribute("title") ??
            active.textContent ??
            ""
          )
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 26),
          top: Math.round(cy),
          bottom: Math.round(cy + box.height),
          left: Math.round(cx),
          right: Math.round(cx + box.width),
          width: box.width,
          height: box.height,
          scroller,
        };

        // A stable id per scroller, so two stops can be told to share one.
        function nextScrollerId() {
          const store = document.documentElement.dataset;
          const next = Number(store.keyboardScrollerSeq ?? "0") + 1;
          store.keyboardScrollerSeq = String(next);
          return next;
        }
      });
      // A zero-size stop is not somewhere a person can see they are.
      if (at.width < 1 || at.height < 1) return;
      walk.push(at);
    });

    stopsPerRoute.push(`${route} → ${walk.length}`);

    stops += walk.length;
    for (let i = 1; i < walk.length; i += 1) {
      const previous = walk[i - 1]!;
      const next = walk[i]!;
      if (previous.scroller !== next.scroller) continue;
      if (previous.height > BAND_HEIGHT || next.height > BAND_HEIGHT) continue;
      if (fullyAbove(next, previous) && !advancedColumn(next, previous)) {
        jumps.push(
          `${route}  "${previous.label}" -> "${next.label}"  up ${previous.top - next.bottom}px`,
        );
      } else if (sameRow(next, previous) && fullyLeft(next, previous)) {
        jumps.push(
          `${route}  "${previous.label}" -> "${next.label}"  left ${previous.left - next.right}px`,
        );
      }
    }
  }

  // A walk that stopped nowhere is in reading order trivially. Printed per route as well,
  // because a route that quietly stops covering its surface is the failure this walk had.
  console.log(`tab stops per route:\n  ${stopsPerRoute.join("\n  ")}`);
  expect(stops, "the walk has to reach real controls").toBeGreaterThan(80);
  expect(jumps, "Tab went backwards against reading order").toEqual([]);
});

// Where focus goes when the thing holding it is removed.
//
// Closing a dock tab from the keyboard is Delete on the focused tab, the ARIA practice for a
// closable tab, and the dock moves focus to the newly active one. That is right until the LAST
// panel closes: there is no tab left, `?.focus()` on nothing is silent, and focus fell to
// `<body>`. A keyboard user does not get a message — their next Tab starts over at the top of
// the document, which is the most complete way to lose someone's place.
//
// Asserted over every close rather than only the last, because the interesting one is whichever
// close happens to empty the dock, and the fixture's tab count is not this test's business.
test("focus survives closing the panel that holds it", async ({ page }) => {
  test.setTimeout(ROUTE_BUDGET_MS);
  await page.goto(`/visual/?fixture=workspace&state=dock-light&theme=light`);
  await page.waitForSelector("html[data-visual-ready]");
  await page.waitForTimeout(400);

  const focused = () =>
    page.evaluate(() => {
      const active = document.activeElement as HTMLElement | null;
      if (!active || active === document.body) return null;
      return (
        active.getAttribute("aria-label") ??
        active.getAttribute("title") ??
        active.textContent ??
        active.tagName
      )
        .trim()
        .replace(/\s+/g, " ")
        .slice(0, 26);
    });

  const onTab = () => page.evaluate(() => document.activeElement?.getAttribute("role") === "tab");

  for (let press = 0; press < 60; press += 1) {
    await page.keyboard.press("Tab");
    await page.waitForTimeout(40);
    if (await onTab()) break;
  }
  expect(await onTab(), "the walk never reached a dock tab").toBe(true);

  const lost: string[] = [];
  let closed = 0;
  while (closed < 12 && (await onTab())) {
    const before = await focused();
    await page.keyboard.press("Delete");
    await page.waitForTimeout(400);
    closed += 1;
    const after = await focused();
    if (after === null) lost.push(`closing "${before}" left focus on <body>`);
  }

  // A run that closed nothing proves nothing, and the last close is the one that matters.
  expect(closed, "no tab was closed").toBeGreaterThan(3);
  expect(await page.locator('[role="tab"]').count(), "the dock never emptied").toBe(0);
  expect(lost, "focus fell out of the document when its tab was removed").toEqual([]);
});
