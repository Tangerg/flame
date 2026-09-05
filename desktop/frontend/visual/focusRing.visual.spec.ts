import { expect, test } from "./test";

// One rule draws every focus ring: 1.5px at `outline-offset: 1px`, so it reaches 2.5px past the
// border box. `[data-focus-inset]` is the compensation for a control flush against something
// that clips — it draws the ring inward instead. The compensation existed, was commented, and
// had **no users**, while the tool-summary disclosure that fills a rounded `overflow-clip`
// container showed a keyboard user no ring at all.
//
// Geometry, not painting: the ring is suppressed unless the last input device was a key
// (`html:not([data-pointer])`), so what is checked is whether it would have anywhere to go.
//
// Two refinements this needed before it said anything true, both about scrolling. An element
// scrolled out of its own container reports a 1000px "cut" and is not a defect, so only a ring
// poking out of a box the ELEMENT fits inside counts. And a scrollable ancestor cannot pin
// anything at all: focus scrolls the control clear of the edge, which is why the walk stops at
// the first scroller instead of blaming the clip beyond it.

const ROUTES = [
  "fixture=agent&state=waiting",
  "fixture=agent&state=running",
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=long-content",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

test("no focus ring is cut off by something that clips", async ({ page }) => {
  const cut: string[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForTimeout(200);

    const found = await page.evaluate(() => {
      const REACH = 2.5;
      const FOCUSABLE =
        'button, a[href], [role="button"], [role="tab"], [role="menuitem"], [role="option"], [role="switch"], [tabindex]:not([tabindex="-1"])';
      const out: string[] = [];
      for (const element of document.querySelectorAll(FOCUSABLE)) {
        if (element.hasAttribute("data-chrome-focus")) continue;
        if (element.hasAttribute("data-focus-inset")) continue;
        const tag = element.tagName.toLowerCase();
        if (tag === "input" || tag === "textarea" || (element as HTMLElement).isContentEditable) {
          continue;
        }
        const style = getComputedStyle(element);
        if (style.visibility === "hidden" || style.display === "none") continue;
        const box = element.getBoundingClientRect();
        if (box.width === 0 || box.height === 0) continue;

        for (let parent = element.parentElement; parent; parent = parent.parentElement) {
          const parentStyle = getComputedStyle(parent);
          // A scrollable ancestor cannot pin anything: focusing scrolls the control clear of
          // the edge, honouring `scroll-padding`. Only a box that CANNOT scroll traps a ring,
          // so the walk stops at the first scroller rather than blaming the clip beyond it.
          const scrolls =
            parentStyle.overflowY === "auto" ||
            parentStyle.overflowY === "scroll" ||
            parentStyle.overflowX === "auto" ||
            parentStyle.overflowX === "scroll";
          if (scrolls) break;
          const clipsX = parentStyle.overflowX !== "visible";
          const clipsY = parentStyle.overflowY !== "visible";
          if (!clipsX && !clipsY) continue;
          const clip = parent.getBoundingClientRect();
          const inside =
            box.top >= clip.top - 0.5 &&
            box.bottom <= clip.bottom + 0.5 &&
            box.left >= clip.left - 0.5 &&
            box.right <= clip.right + 0.5;
          if (!inside) continue;
          const bleed = Math.max(
            clipsY ? clip.top - (box.top - REACH) : 0,
            clipsY ? box.bottom + REACH - clip.bottom : 0,
            clipsX ? clip.left - (box.left - REACH) : 0,
            clipsX ? box.right + REACH - clip.right : 0,
          );
          if (bleed <= 0.25) continue;
          out.push(
            `${Math.round(bleed * 10) / 10}px cut from <${tag} class="${(element.getAttribute("class") ?? "").slice(0, 48)}"> ` +
              `by ${parent.tagName.toLowerCase()}.${(parent.getAttribute("class") ?? "").slice(0, 36)}`,
          );
          break;
        }
      }
      return out;
    });
    cut.push(...found);
  }

  expect(
    [...new Set(cut)],
    "focus rings with nowhere to draw — mark the control `data-focus-inset`",
  ).toEqual([]);
});
