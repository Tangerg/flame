import { expect, test, type Page } from "@playwright/test";

// The design rules ask of every surface: does it get squeezed when the window is narrow?
// Nothing asserted it. The invariant a person actually feels is narrower than "no overflow" —
// wide blocks and the message action bar overhang the reading column ON PURPOSE, and that is
// fine because nothing clips them. What is never fine is a control the window cuts in half:
// it is unreadable, and the half outside cannot be clicked.
//
// The viewport is the configured minimum, so this is the tightest the shell is ever asked to be.

const FIXTURES = [
  "fixture=agent&theme=light&state=long-content",
  "fixture=agent&theme=light&state=tool-shells",
  "fixture=agent&theme=light&state=narrative",
  "fixture=agent&theme=light&state=question",
  "fixture=workspace&theme=light&state=dock-light",
  "fixture=workspace&theme=light&state=settings",
  "fixture=shell&theme=light&state=populated",
];

const INTERACTIVE = 'button, a[href], input, textarea, select, [role="tab"], [role="option"]';

interface Clipped {
  label: string;
  cutBy: string;
  pixels: number;
}

async function clippedControls(page: Page, selector: string): Promise<Clipped[]> {
  return page.evaluate((interactive) => {
    const out: Clipped[] = [];
    for (const el of document.querySelectorAll<HTMLElement>(interactive)) {
      const style = getComputedStyle(el);
      if (style.display === "none" || style.visibility === "hidden" || style.opacity === "0") {
        continue;
      }
      const box = el.getBoundingClientRect();
      if (box.width < 1 || box.height < 1) continue;
      for (let parent = el.parentElement; parent; parent = parent.parentElement) {
        const parentStyle = getComputedStyle(parent);
        const clips = parentStyle.overflowX !== "visible" || parentStyle.overflowY !== "visible";
        if (!clips) continue;
        const edge = parent.getBoundingClientRect();
        // Whether the person can REACH the rest, which is the overflow VALUE and not the
        // scroll dimensions: a `hidden` box reports scrollWidth > clientWidth exactly when it
        // is cutting something off, so measuring that would excuse every defect this looks
        // for. `auto`/`scroll` is allowed — a dock tab strip scrolls sideways and its last tab
        // is half-visible by design.
        const reachable = (value: string) => value === "auto" || value === "scroll";
        const scrollable = reachable(parentStyle.overflowX) || reachable(parentStyle.overflowY);
        const cutLeft = edge.left - box.left;
        const cutRight = box.right - edge.right;
        const worst = Math.max(cutLeft, cutRight);
        if (worst > 1 && worst < box.width - 1 && !scrollable) {
          out.push({
            label:
              el.getAttribute("aria-label") ?? el.textContent?.trim().slice(0, 40) ?? el.tagName,
            cutBy: String(parent.className).slice(0, 60),
            pixels: Math.round(worst),
          });
        }
        break;
      }
    }
    return out;
  }, selector);
}

for (const query of FIXTURES) {
  test(`no control is cut in half at the minimum window: ${query}`, async ({ page }) => {
    await page.goto(`/visual/?${query}`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(400);

    // Not vacuous: the sweep has to be looking at real controls.
    const considered = await page.locator(INTERACTIVE).count();
    expect(considered).toBeGreaterThan(3);

    expect(await clippedControls(page, INTERACTIVE)).toEqual([]);
  });
}
