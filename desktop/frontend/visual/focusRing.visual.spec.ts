import { expect, test } from "./test";
import { FOCUSABLE } from "./controls";
import { eachTabStop } from "./tabWalk";

const ROUTES = [
  "fixture=agent&state=waiting",
  "fixture=agent&state=running",
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=long-content",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=shell&state=populated&overlay=finder",
  "fixture=shell&state=populated&overlay=commands",
];

test("no focus ring is cut off by something that clips", async ({ page }) => {
  const cut: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);
    reached += await page.locator(FOCUSABLE).count();

    const found = await page.evaluate((FOCUSABLE) => {
      const REACH = 2.5;
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
    }, FOCUSABLE);
    cut.push(...found);
  }

  expect(reached, "the sweep has to be looking at real controls").toBeGreaterThan(20);
  expect(
    [...new Set(cut)],
    "focus rings with nowhere to draw — mark the control `data-focus-inset`",
  ).toEqual([]);
});

const ROUTE_BUDGET_MS = 120_000;

test("the ring the design promises is the ring that paints", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const silent: string[] = [];
  const strangers: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    await eachTabStop(page, async () => {
      const meta = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement;
        const tag = active.tagName.toLowerCase();
        if (active.hasAttribute("data-chrome-focus")) return null;
        if (tag === "input" || tag === "textarea" || active.isContentEditable) return null;
        if (!active.matches(":focus-visible")) return null;
        const style = getComputedStyle(active);

        const painters: string[] = [];
        for (const sheet of document.styleSheets) {
          let rules: CSSRuleList;
          try {
            rules = sheet.cssRules;
          } catch {
            continue;
          }
          const walk = (list: CSSRuleList) => {
            for (const rule of list) {
              if (rule instanceof CSSGroupingRule) walk(rule.cssRules);
              if (!(rule instanceof CSSStyleRule)) continue;
              if (!/outline/.test(rule.style.cssText)) continue;
              let hit = false;
              try {
                hit = active.matches(rule.selectorText);
              } catch {
                hit = false;
              }
              if (hit) painters.push(rule.selectorText.replace(/\s+/g, " "));
            }
          };
          walk(rules);
        }
        return {
          tag,
          painters,
          drawn: style.outlineStyle !== "none",
          label: (active.getAttribute("aria-label") ?? active.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 34),
        };
      });
      if (!meta) return;
      reached += 1;
      if (!meta.drawn) {
        silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
        return;
      }
      const foreign = meta.painters.filter(
        (selector) => !(selector.includes("data-pointer") && selector.includes(":focus-visible")),
      );
      if (foreign.length > 0 || meta.painters.length === 0) {
        strangers.push(
          `${route} <${meta.tag}> "${meta.label}"  ${foreign.length > 0 ? foreign.join(" ; ") : "no author rule paints it, so this is the browser's own"}`,
        );
      }
    });
  }

  expect(reached, "the walk has to arrive at real controls").toBeGreaterThan(60);
  expect(
    [...new Set(silent)],
    "controls the design promises a ring and that show none — a call site is out-specifying globals.css",
  ).toEqual([]);
  expect(
    [...new Set(strangers)],
    "one rule draws every focus ring, so nothing else may paint one",
  ).toEqual([]);
});
