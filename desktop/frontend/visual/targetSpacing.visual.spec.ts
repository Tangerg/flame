import { expect, test } from "./test";
import { CONTROL } from "./controls";

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=agent&state=narrative",
  "fixture=agent&state=waiting",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=delegated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=dock-timeline",
  "fixture=workspace&state=settings",
];

test("an undersized control keeps 24px of clearance from its neighbours", async ({ page }) => {
  const crowded: string[] = [];
  let undersized = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(220);

    const result = await page.evaluate((SELECTOR) => {
      const boxes = [...document.querySelectorAll(SELECTOR)]
        .filter((el) => !el.closest("[data-fixture-chrome]"))
        .filter((el) => {
          const style = getComputedStyle(el);
          const r = el.getBoundingClientRect();
          return r.width > 2 && r.height > 2 && style.visibility !== "hidden";
        })
        .map((el) => ({ el, r: el.getBoundingClientRect() }));
      const label = (el: Element) =>
        (el.getAttribute("aria-label") || el.textContent || el.tagName)
          .trim()
          .replace(/\s+/g, " ")
          .slice(0, 24);

      let small = 0;
      const out: string[] = [];
      for (const { el, r } of boxes) {
        if (r.width >= 24 && r.height >= 24) continue;
        if (el.closest('[data-slot="chat-rail"]')) continue;
        small += 1;
        const cx = r.left + r.width / 2;
        const cy = r.top + r.height / 2;
        for (const other of boxes) {
          if (other.el === el || other.el.contains(el) || el.contains(other.el)) continue;
          const nx = Math.max(other.r.left, Math.min(cx, other.r.right));
          const ny = Math.max(other.r.top, Math.min(cy, other.r.bottom));
          if (Math.hypot(nx - cx, ny - cy) < 12) {
            out.push(`"${label(el)}" <-> "${label(other.el)}"`);
            break;
          }
        }
      }
      return { small, out };
    }, CONTROL);
    undersized += result.small;
    crowded.push(...result.out.map((pair) => `${route}  ${pair}`));
  }

  expect(undersized, "the sweep has to be looking at undersized controls").toBeGreaterThan(3);
  expect(crowded, "undersized controls closer than 24px to a neighbour").toEqual([]);
});
