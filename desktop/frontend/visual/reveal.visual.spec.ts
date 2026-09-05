import { expect, test } from "./test";

// An element at `opacity: 0` is invisible and still takes clicks. Eight of this app's nine
// hover-reveals had only the opacity, so every one of them was an invisible target sitting on
// top of something else — the largest a 774x26 strip of message actions, and one of them the
// action on every Work Index row, which a click near the row's right end would hit instead of
// the row. `MessageBlock` had the guard on its `hidden` variant and not on its `hover` one,
// two adjacent lines apart, which is what says nobody decided they should differ.
//
// `[data-reveal="hover"]` now carries `pointer-events: none` at rest, so this check is what
// keeps a reveal from re-opening the hole: a new spelling either restores hit-testing in the
// same variant that restores opacity, or shows up here.

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=narrative",
  "fixture=agent&state=waiting",
  "fixture=agent&state=long-content",
  "fixture=workspace&state=dock-tools",
];

test("nothing invisible can be clicked", async ({ page }) => {
  const traps: string[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForTimeout(200);

    const found = await page.evaluate(() => {
      const out: string[] = [];
      for (const element of document.querySelectorAll('[data-reveal="hover"]')) {
        const style = getComputedStyle(element);
        const box = element.getBoundingClientRect();
        if (box.width === 0 || box.height === 0) continue;
        const invisible = style.opacity === "0" || style.visibility === "hidden";
        const clickable = style.pointerEvents !== "none" && style.visibility !== "hidden";
        if (!invisible || !clickable) continue;
        out.push(
          `${Math.round(box.width)}x${Math.round(box.height)} ` +
            `<${element.tagName.toLowerCase()} class="${(element.getAttribute("class") ?? "").slice(0, 60)}">`,
        );
      }
      return out;
    });
    traps.push(...found.map((entry) => `${route}  ${entry}`));
  }

  expect([...new Set(traps)], "invisible elements that still take clicks").toEqual([]);
});
