import { expect, test } from "./test";
import { eachTabStop } from "./tabWalk";

const ROUTES = [
  "fixture=agent&state=question",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

const SETTLE_MS = 500;
const ROUTE_BUDGET_MS = 120_000;

test("a control that turns off the ring shows focus some other way", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const silent: string[] = [];
  let optOuts = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");

    await eachTabStop(page, async () => {
      const meta = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement;
        if (!active.hasAttribute("data-chrome-focus")) return null;
        if (!active.matches(":focus-visible")) return null;
        const box = active.getBoundingClientRect();
        if (box.width === 0 || box.y < 0 || box.y + box.height > 720) return null;
        if (box.x + box.width > 1120) return null;
        active.dataset.chromeFocusProbe = "";
        return {
          label: (
            active.getAttribute("aria-label") ??
            active.getAttribute("title") ??
            active.textContent ??
            ""
          )
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: active.tagName.toLowerCase(),
          clip: {
            x: Math.max(0, box.x - 3),
            y: Math.max(0, box.y - 3),
            width: box.width + 6,
            height: box.height + 6,
          },
        };
      });
      if (!meta) return;
      optOuts += 1;

      const focused = await page.screenshot({ clip: meta.clip });
      await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
      await page.waitForTimeout(SETTLE_MS);
      const blurred = await page.screenshot({ clip: meta.clip });
      if (Buffer.compare(focused, blurred) === 0) {
        silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
      }

      await page.evaluate(() => {
        const probe = document.querySelector<HTMLElement>("[data-chrome-focus-probe]");
        probe?.focus();
        probe?.removeAttribute("data-chrome-focus-probe");
      });
    });
  }

  expect(optOuts, "the walk has to reach controls that opted out").toBeGreaterThan(8);
  expect(
    [...new Set(silent)],
    "controls with `data-chrome-focus` that show nothing when a keyboard reaches them",
  ).toEqual([]);
});
