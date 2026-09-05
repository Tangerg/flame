import { expect, test } from "./test";

// `@media (pointer: coarse)` puts a 44px floor under every control, and two spans in
// `globals.css` reserve room for exactly one chrome control each. The spans were written as
// `36px` — the 26px control plus its inset and clearance — so on a touch screen the control
// grew and the reservation did not, and the dock's browse button and its collapse control
// overlapped by exactly the difference. Both spans derive from the control now.
//
// Nothing else in the suite runs with a coarse pointer: `@media (hover: none)` also forces
// every hover-reveal permanently visible there, so touch is a layout the desktop tests never
// see. Emulated with a touch CONTEXT — `Emulation.setEmulatedMedia` does not carry `hover` or
// `pointer`, and a probe using it measured nothing while appearing to work.
//
// Containment is the discriminator, not a list of known pairs: a row action sits ON its row by
// design and wins inside its own box, so one box inside the other is composition. Two boxes
// that merely intersect are neighbours competing for the same tap.

test.use({ hasTouch: true, isMobile: true });

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=agent&state=waiting",
  "fixture=agent&state=tool-shells",
  "fixture=workspace&state=dock-tools",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

test("no two neighbouring controls share a tap under a coarse pointer", async ({ page }) => {
  const overlapping: string[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(220);

    const coarse = await page.evaluate(() => window.matchMedia("(pointer: coarse)").matches);
    expect(coarse, "the context must actually report a coarse pointer").toBe(true);

    const pairs = await page.evaluate(() => {
      const SELECTOR =
        'button, a[href], input:not([type="hidden"]), textarea, select, [role="button"], [role="tab"], [role="menuitem"], [role="option"], [role="switch"]';
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
    });
    overlapping.push(...pairs.map((pair) => `${route}  ${pair}`));
  }

  expect(
    [...new Set(overlapping)],
    "neighbouring controls whose tap targets intersect on a touch screen",
  ).toEqual([]);
});
