import { expect, test } from "./test";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";

const ROUTES = [
  ...VISUAL_AGENT_STATES.map((state) => `fixture=agent&state=${state}`),
  "fixture=workspace&state=dock-timeline",
  "fixture=workspace&state=dock-runs",
  "fixture=workspace&state=dock-subagents",
  "fixture=workspace&state=settings",
];

const ROUTE_BUDGET_MS = 4_000;

test("a control that says it operates a region operates one that exists", async ({ page }) => {
  test.setTimeout(ROUTES.length * ROUTE_BUDGET_MS + 20_000);
  const dangling: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);

    const found = await page.evaluate(() => {
      const out: string[] = [];
      let seen = 0;
      for (const element of document.querySelectorAll("[aria-expanded][aria-controls]")) {
        seen += 1;
        const id = element.getAttribute("aria-controls") ?? "";
        if (document.getElementById(id)) continue;
        out.push(
          (element.getAttribute("aria-label") ?? element.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 40),
        );
      }
      return { out, seen };
    });
    reached += found.seen;
    dangling.push(...found.out.map((label) => `${route}  "${label}"`));
  }

  expect(reached, "the sweep has to find real disclosures").toBeGreaterThan(12);
  expect(
    [...new Set(dangling)],
    "`aria-controls` naming an id nothing carries — the region is deferred, its identity is not",
  ).toEqual([]);
});
