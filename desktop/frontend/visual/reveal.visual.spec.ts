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

// The other half of the same idea: a reveal has TWO ends. `globals.css` says it — "`rest` is
// the other end of it: what the reveal displaces, which has to give way at the same moment or
// the two overlap" — and nothing checked the two were driven by the same condition. They were
// not: the resting glyph watched the TRIGGER's `:focus-visible` while the action watched the
// row's `:focus-within`, so focus landing on the action itself left the row showing both.
//
// Two mechanisms carry a pair here, `[data-reveal]` and `.t-icon-swap`'s `[data-glyph]`, and
// both are asked the same question in each state a person can put the row in.
const PAIRS = [
  { rest: '[data-reveal="rest"]', shown: '[data-reveal="hover"]', within: ".group\\/row" },
  { rest: '[data-glyph="rest"]', shown: '[data-glyph="hover"]', within: ".t-icon-swap" },
];

test("a reveal and the thing it displaces never show at once", async ({ page }) => {
  const both: string[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");

    for (const pair of PAIRS) {
      const containers = await page.locator(pair.within).elementHandles();
      for (const container of containers.slice(0, 6)) {
        const box = await container.boundingBox();
        if (!box || box.width === 0) continue;

        const sample = async (state: string) => {
          const reading = await container.evaluate(
            (node: Element, selectors) => {
              const rest = node.querySelector(selectors.rest);
              const shown = node.querySelector(selectors.shown);
              if (!rest || !shown) return null;
              return {
                rest: Number(getComputedStyle(rest).opacity),
                shown: Number(getComputedStyle(shown).opacity),
              };
            },
            { rest: pair.rest, shown: pair.shown },
          );
          // Both substantially visible is the failure; a cross-fade mid-flight is not.
          if (reading && reading.rest > 0.6 && reading.shown > 0.6) {
            both.push(
              `${route} ${state}: rest=${reading.rest} shown=${reading.shown} in ${pair.within}`,
            );
          }
        };

        await sample("at rest");
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
        await page.waitForTimeout(220);
        await sample("hovered");
        // The pointer has to leave first, or `:hover` retires the resting end and hides the
        // disagreement this step exists to find.
        await page.mouse.move(0, 0);
        await page.waitForTimeout(180);
        // The REVEALED end, not the first control in the row: focus landing on the action is
        // the state the two ends disagreed about, and focusing the trigger hides the
        // disagreement because both conditions happen to hold there.
        await container.evaluate((node: Element, selector) => {
          const revealed = node.querySelector<HTMLElement>(selector);
          (revealed?.querySelector<HTMLElement>("button, [tabindex]") ?? revealed)?.focus();
        }, pair.shown);
        await page.waitForTimeout(220);
        await sample("the revealed end focused");
        await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
        await page.waitForTimeout(180);
      }
    }
  }

  expect([...new Set(both)], "a reveal showing at the same time as what it displaces").toEqual([]);
});
