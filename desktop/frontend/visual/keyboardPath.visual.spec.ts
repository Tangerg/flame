import { expect, test } from "./test";
import { eachTabStop } from "./tabWalk";

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
];

const BAND_HEIGHT = 400;

const fullyAbove = (next: Box, previous: Box) => next.bottom <= previous.top;
const fullyLeft = (next: Box, previous: Box) => next.right <= previous.left;
const sameRow = (next: Box, previous: Box) =>
  next.top < previous.bottom && previous.top < next.bottom;
const advancedColumn = (next: Box, previous: Box) => next.left >= previous.right;

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

        function nextScrollerId() {
          const store = document.documentElement.dataset;
          const next = Number(store.keyboardScrollerSeq ?? "0") + 1;
          store.keyboardScrollerSeq = String(next);
          return next;
        }
      });
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

  console.log(`tab stops per route:\n  ${stopsPerRoute.join("\n  ")}`);
  expect(stops, "the walk has to reach real controls").toBeGreaterThan(80);
  expect(jumps, "Tab went backwards against reading order").toEqual([]);
});

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

  expect(closed, "no tab was closed").toBeGreaterThan(3);
  expect(await page.locator('[role="tab"]').count(), "the dock never emptied").toBe(0);
  expect(lost, "focus fell out of the document when its tab was removed").toEqual([]);
});
