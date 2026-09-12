import { expect, test } from "./test";

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

const PAIRS = [
  { rest: '[data-reveal="rest"]', shown: '[data-reveal="hover"]', label: "row action" },
  { rest: '[data-glyph="rest"]', shown: '[data-glyph="hover"]', label: "icon swap" },
];

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
        await page.mouse.move(0, 0);
        await page.waitForTimeout(180);
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

  for (const pair of PAIRS) {
    expect(
      byPair.get(pair.label) ?? 0,
      `no ${pair.label} pair was found on any route`,
    ).toBeGreaterThan(0);
  }
  expect(examined, "the sweep has to put a real container through its states").toBeGreaterThan(1);
  expect([...new Set(both)], "a reveal showing at the same time as what it displaces").toEqual([]);
});
