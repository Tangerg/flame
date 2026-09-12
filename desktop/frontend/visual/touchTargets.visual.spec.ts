import { expect, test } from "./test";
import { CONTROL } from "./controls";

test.use({ hasTouch: true, isMobile: true });

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=agent&state=waiting",
  "fixture=agent&state=tool-shells",
  "fixture=workspace&state=dock-tools",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=shell&state=populated&overlay=finder",
  "fixture=shell&state=populated&overlay=commands",
];

test("no two neighbouring controls share a tap under a coarse pointer", async ({ page }) => {
  const overlapping: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(220);

    const coarse = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
    expect(coarse, "the context must actually report a coarse pointer").toBe(true);

    reached += await page.locator(CONTROL).count();

    const pairs = await page.evaluate((SELECTOR) => {
      const boxes: {
        el: Element;
        left: number;
        top: number;
        right: number;
        bottom: number;
        label: string;
      }[] = [];
      for (const el of document.querySelectorAll(SELECTOR)) {
        const style = getComputedStyle(el);
        if (style.visibility === "hidden" || style.display === "none") continue;
        if (style.pointerEvents === "none") continue;
        const r = el.getBoundingClientRect();
        let box = { left: r.left, top: r.top, right: r.right, bottom: r.bottom };
        for (let parent = el.parentElement; parent; parent = parent.parentElement) {
          const ps = getComputedStyle(parent);
          if (ps.overflowX === "visible" && ps.overflowY === "visible") continue;
          const pr = parent.getBoundingClientRect();
          box = {
            left: Math.max(box.left, pr.left),
            top: Math.max(box.top, pr.top),
            right: Math.min(box.right, pr.right),
            bottom: Math.min(box.bottom, pr.bottom),
          };
        }
        if (box.right - box.left <= 0 || box.bottom - box.top <= 0) continue;
        boxes.push({
          el,
          ...box,
          label: (el.getAttribute("aria-label") || el.textContent || "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 24),
        });
      }

      const out: string[] = [];
      const contains = (outer: (typeof boxes)[number], inner: (typeof boxes)[number]) =>
        outer.left <= inner.left + 1 &&
        outer.top <= inner.top + 1 &&
        outer.right >= inner.right - 1 &&
        outer.bottom >= inner.bottom - 1;

      for (let i = 0; i < boxes.length; i += 1) {
        for (let j = i + 1; j < boxes.length; j += 1) {
          const a = boxes[i]!;
          const b = boxes[j]!;
          if (a.el.contains(b.el) || b.el.contains(a.el)) continue;
          if (contains(a, b) || contains(b, a)) continue;
          const ox = Math.min(a.right, b.right) - Math.max(a.left, b.left);
          const oy = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
          if (ox > 1 && oy > 1) {
            out.push(`${Math.round(ox)}x${Math.round(oy)}px  "${a.label}" <-> "${b.label}"`);
          }
        }
      }
      return out;
    }, CONTROL);
    overlapping.push(...pairs.map((pair) => `${route}  ${pair}`));
  }

  expect(reached, "the sweep has to be looking at real controls").toBeGreaterThan(20);
  expect(
    [...new Set(overlapping)],
    "neighbouring controls whose tap targets intersect on a touch screen",
  ).toEqual([]);
});
