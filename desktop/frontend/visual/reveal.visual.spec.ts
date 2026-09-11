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
    await page.waitForSelector("html[data-visual-ready]");
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
// THE CONTAINER IS FOUND BY STRUCTURE, and that is the correction. This looked for `.group/row`,
// which was a Tailwind group name and left with Tailwind: measured across all six routes, it
// matched ZERO elements, so the pair the defect was actually found in has been reaching nothing
// for as long as this has been green. The two ends are siblings — the resting glyph lives inside
// the row's own button and the action beside it — so the holder is an unnamed wrapper, and
// naming it again would only set the same trap. Walking up from each `rest` to the nearest
// ancestor that also holds a `shown` cannot be renamed out from under this.
const PAIRS = [
  { rest: '[data-reveal="rest"]', shown: '[data-reveal="hover"]', label: "row action" },
  { rest: '[data-glyph="rest"]', shown: '[data-glyph="hover"]', label: "icon swap" },
];

/** Marks every rest/shown pair holder on the page and returns how many there are. */
async function markPairs(
  page: import("@playwright/test").Page,
  pair: { rest: string; shown: string },
): Promise<number> {
  return page.evaluate((selectors) => {
    let found = 0;
    for (const node of document.querySelectorAll("[data-reveal-pair]")) {
      node.removeAttribute("data-reveal-pair");
    }
    for (const rest of document.querySelectorAll(selectors.rest)) {
      for (let node = rest.parentElement; node; node = node.parentElement) {
        if (!node.querySelector(selectors.shown)) continue;
        if (!node.hasAttribute("data-reveal-pair")) {
          node.setAttribute("data-reveal-pair", String(found));
          found += 1;
        }
        break;
      }
    }
    return found;
  }, pair);
}

test("a reveal and the thing it displaces never show at once", async ({ page }) => {
  test.setTimeout(ROUTES.length * 40_000 + 30_000);
  const both: string[] = [];
  let examined = 0;
  const byPair = new Map<string, number>();

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    for (const pair of PAIRS) {
      const count = await markPairs(page, pair);
      byPair.set(pair.label, (byPair.get(pair.label) ?? 0) + count);

      for (let index = 0; index < Math.min(count, 6); index += 1) {
        const container = page.locator(`[data-reveal-pair="${index}"]`);
        const box = await container.boundingBox();
        if (!box || box.width === 0) continue;
        examined += 1;

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
              `${route} ${pair.label} ${state}: rest=${reading.rest} shown=${reading.shown}`,
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

  // Floors, and the reason this file now has them: the `.group/row` selector matched nothing on
  // any route and the test reported green the whole time. Each pair is floored separately, so a
  // mechanism that stops appearing is named rather than absorbed by the other one's count.
  for (const pair of PAIRS) {
    expect(
      byPair.get(pair.label) ?? 0,
      `no ${pair.label} pair was found on any route`,
    ).toBeGreaterThan(0);
  }
  expect(examined, "the sweep has to put a real container through its states").toBeGreaterThan(1);
  expect([...new Set(both)], "a reveal showing at the same time as what it displaces").toEqual([]);
});
