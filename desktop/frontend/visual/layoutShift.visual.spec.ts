import { expect, test, type Page } from "./test";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES } from "./shellFixtureStates";
import { VISUAL_WORKSPACE_STATES } from "./workspaceFixtureStates";

// A golden is taken after everything settles, so a jump on the way there leaves no trace in it.
// `layout-shift` is the browser's own record of that jump — the entries CLS is computed from —
// and it names the element that moved and where it moved from.
//
// The baseline is FIRST CONTENTFUL PAINT, not first paint, and the distinction is the whole
// check. Two shifts land before content exists and neither is visible: the dev server delivers
// CSS through the module graph, so the first paint is an unstyled `#root` that then settles
// (production links the stylesheet in `<head>`, where it blocks paint and the shift cannot
// happen), and React's mount pass moves rows into a transcript nobody has seen yet. Measuring
// from first paint reports every state as jumping and points the fix at production code that is
// already correct.
//
// What survives the baseline is a real one: content the user is reading moved under them,
// typically a measurement published from `useEffect` where layout order demanded
// `useLayoutEffect`, or an image given no width to reserve.

const OBSERVER = `
  window.__shifts = [];
  window.__contentPaintedAt = Infinity;
  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      if (entry.name === "first-contentful-paint") window.__contentPaintedAt = entry.startTime;
    }
  }).observe({ type: "paint", buffered: true });
  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      if (entry.hadRecentInput) continue;
      if (entry.startTime <= window.__contentPaintedAt) continue;
      window.__shifts.push({
        value: entry.value,
        moved: (entry.sources ?? []).map((source) => {
          const node = source.node;
          const label = node
            ? \`<\${node.tagName?.toLowerCase() ?? "?"} \${node.getAttribute?.("data-slot") ?? node.getAttribute?.("class")?.slice(0, 40) ?? ""}>\`
            : "<detached>";
          return \`\${label} y \${Math.round(source.previousRect?.y ?? 0)} -> \${Math.round(source.currentRect?.y ?? 0)}\`;
        }),
      });
    }
  }).observe({ type: "layout-shift", buffered: true });
`;

interface Shift {
  value: number;
  moved: string[];
}

async function settleAndCollect(page: Page, route: string): Promise<Shift[]> {
  await page.goto(`/visual/?${route}&theme=light`);
  await page.waitForSelector("html[data-visual-ready]");
  await page.waitForTimeout(700);
  return page.evaluate(() => (window as unknown as { __shifts: Shift[] }).__shifts ?? []);
}

function report(entries: { route: string; shift: Shift }[]): string {
  return entries
    .map(
      ({ route, shift }) =>
        `\n  ${route}  shifted ${shift.value.toFixed(4)}\n     ${shift.moved.join("\n     ")}`,
    )
    .join("");
}

async function expectSettled(page: Page, routes: string[]) {
  await page.addInitScript(OBSERVER);
  const found: { route: string; shift: Shift }[] = [];
  for (const route of routes) {
    for (const shift of await settleAndCollect(page, route)) found.push({ route, shift });
  }
  expect(found, report(found)).toEqual([]);
}

test("agent states paint their content once", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_AGENT_STATES.map((state) => `fixture=agent&state=${state}`),
  );
});

test("work index states paint their content once", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_WORK_INDEX_STATES.map((state) => `fixture=shell&state=${state}`),
  );
});

test("workspace states paint their content once", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_WORKSPACE_STATES.map((state) => `fixture=workspace&state=${state}`),
  );
});

test("a jump after content is painted is caught", async ({ page }) => {
  await page.addInitScript(OBSERVER);
  await page.goto("/visual/?fixture=agent&state=narrative&theme=light");
  await page.waitForSelector("html[data-visual-ready]");
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    const spacer = document.createElement("div");
    spacer.style.height = "80px";
    document.body.prepend(spacer);
  });
  await page.waitForTimeout(200);

  const shifts = await page.evaluate(
    () => (window as unknown as { __shifts: Shift[] }).__shifts ?? [],
  );
  expect(shifts.length).toBeGreaterThan(0);
});
