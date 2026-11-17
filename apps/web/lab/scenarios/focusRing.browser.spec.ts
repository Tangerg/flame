import { expect, test } from "./test";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";
import { FOCUSABLE } from "./controls";
import { eachTabStop } from "./tabWalk";
import { expectLabReady } from "../readiness";
const ROUTES = [
  ...VISUAL_AGENT_STATES.map((state) => `fixture=agent&state=${state}`),
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=shell&state=populated&overlay=finder",
  "fixture=shell&state=populated&overlay=commands",
];

const ROUTE_BUDGET_MS = 120_000;

test("no focus ring is cut off by something that clips", async ({ page }) => {
  test.setTimeout(ROUTES.length * 4_000 + 20_000);
  const cut: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/lab/fixture.html?${route}&theme=light`);
    await expectLabReady(page);
    await page.waitForTimeout(200);
    reached += await page.locator(FOCUSABLE).count();

    const found = await page.evaluate((FOCUSABLE) => {
      const REACH = 2.5;
      const out: string[] = [];
      for (const element of document.querySelectorAll(FOCUSABLE)) {
        if (element.hasAttribute("data-chrome-focus")) continue;
        if (element.closest("[data-focus-inset]")) continue;
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

test("keyboard tab focus remains visibly indicated", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const silent: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/lab/fixture.html?${route}&theme=light`);
    await expectLabReady(page);
    await page.waitForTimeout(200);

    await eachTabStop(page, async () => {
      const meta = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement;
        const tag = active.tagName.toLowerCase();
        const proxy = active.closest<HTMLElement>("[data-focus-proxy]");
        if (active.hasAttribute("data-chrome-focus") && !proxy) return null;
        if ((tag === "input" || tag === "textarea" || active.isContentEditable) && !proxy)
          return null;
        if (!active.matches(":focus-visible")) return null;
        const style = getComputedStyle(proxy ?? active);
        return {
          tag,
          drawn: style.outlineStyle !== "none",
          label: (active.getAttribute("aria-label") ?? active.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 34),
        };
      });
      if (!meta) return;
      reached += 1;
      if (!meta.drawn) silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
    });
  }

  expect(reached, "the walk has to arrive at real controls").toBeGreaterThan(60);
  expect([...new Set(silent)], "keyboard-reached controls without a visible focus outline").toEqual(
    [],
  );
});
