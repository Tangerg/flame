import { expect, test, type Page } from "./test";
import { CONTROL } from "./controls";

// Narrower than "no overflow": wide blocks and the message action bar overhang the reading
// column on purpose, and nothing clips them. What is never fine is a control cut in half — the
// part outside cannot be clicked. The viewport is the configured minimum.

const FIXTURES = [
  "fixture=agent&theme=light&state=long-content",
  "fixture=agent&theme=light&state=tool-shells",
  "fixture=agent&theme=light&state=narrative",
  "fixture=agent&theme=light&state=question",
  "fixture=workspace&theme=light&state=dock-light",
  "fixture=workspace&theme=light&state=settings",
  "fixture=shell&theme=light&state=populated",
  "fixture=shell&theme=light&state=populated&overlay=finder",
  "fixture=shell&theme=light&state=populated&overlay=commands",
];

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
        // Reachability is the overflow VALUE, not the scroll dimensions: a `hidden` box reports
        // scrollWidth > clientWidth exactly when it is cutting something off, so measuring that
        // would excuse every defect this looks for.
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
    const considered = await page.locator(CONTROL).count();
    expect(considered).toBeGreaterThan(3);

    expect(await clippedControls(page, CONTROL)).toEqual([]);
  });
}
