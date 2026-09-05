import { expect, test, type Page } from "./test";
import { VISUAL_AGENT_STATES } from "./agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES } from "./shellFixtureStates";
import { VISUAL_WORKSPACE_STATES } from "./workspaceFixtureStates";

// A golden is taken after everything settles, so a jump on the way there leaves no trace in it.
// `layout-shift` is the browser's own record of that jump — the entries CLS is computed from —
// and it names the element that moved and where it moved from.
//
// The baseline is the fixture's own READY signal, and choosing it is the whole check. Three
// earlier candidates all measure the wrong thing:
//
//   * first paint — the dev server delivers CSS through the module graph, so the first frame is
//     an unstyled `#root` that then settles. Production links the stylesheet in `<head>`, where
//     it blocks paint and the shift cannot happen. Every state "jumps", and the fix would be
//     pointed at production code that is already correct.
//   * first CONTENTFUL paint — closer, and wrong in a way that only shows up as a flake: it is a
//     browser milestone racing the app's own staged loading. `dock-tools` fills in waves, and
//     whether its second wave lands before or after that milestone decides whether the run passes.
//     Measured: three failures in four runs, then none in the next.
//   * "no `aria-busy` left" — the dock keeps skeletons a fixture seeds no data for, so it never
//     arrives.
//
// Ready is the app's own claim that it has finished, which is the only line a jump can be judged
// against: before it the user is watching something load, after it they are reading. What
// survives is a real one — a measurement published from `useEffect` where layout order demanded
// `useLayoutEffect`, or an image given no width to reserve.

const OBSERVER = `
  window.__shifts = [];
  window.__readyAt = null;

  // The init script runs before the document has an element to observe, so arming waits for one.
  // Nothing here filters: every shift is kept with its timestamp and the cut is made once the
  // ready stamp is known, so an observer armed a beat late cannot silently drop or keep the wrong
  // side of the line.
  const stamp = () => {
    if (window.__readyAt === null) window.__readyAt = performance.now();
  };
  const arm = () => {
    const root = document.documentElement;
    if (!root) {
      setTimeout(arm, 0);
      return;
    }
    if (root.hasAttribute("data-visual-ready")) {
      stamp();
      return;
    }
    new MutationObserver((_, observer) => {
      if (root.hasAttribute("data-visual-ready")) {
        stamp();
        observer.disconnect();
      }
    }).observe(root, { attributes: true });
  };
  arm();

  new PerformanceObserver((list) => {
    for (const entry of list.getEntries()) {
      if (entry.hadRecentInput) continue;
      window.__shifts.push({
        at: entry.startTime,
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
  at: number;
  value: number;
  moved: string[];
}

/** Everything the page recorded, cut at the moment the app said it was ready. Throws rather than
 *  reports nothing when the stamp is missing: an instrument that never armed reads exactly like a
 *  page that never moved, and this one has already failed that way once. */
async function shiftsAfterReady(page: Page): Promise<Shift[]> {
  const { shifts, readyAt } = await page.evaluate(() => {
    const w = window as unknown as { __shifts: Shift[]; __readyAt: number | null };
    return { shifts: w.__shifts ?? [], readyAt: w.__readyAt };
  });
  if (readyAt === null) throw new Error("the ready stamp never landed — the observer did not arm");
  return shifts.filter((shift) => shift.at > readyAt);
}

async function settleAndCollect(page: Page, route: string): Promise<Shift[]> {
  await page.goto(`/visual/?${route}&theme=light`);
  await page.waitForSelector("html[data-visual-ready]");
  await page.waitForTimeout(700);
  return shiftsAfterReady(page);
}

function report(entries: { route: string; shift: Shift }[]): string {
  return entries
    .map(
      ({ route, shift }) =>
        `\n  ${route}  shifted ${shift.value.toFixed(4)}\n     ${shift.moved.join("\n     ")}`,
    )
    .join("");
}

// Each route is a page load plus the settle window, and the state lists only grow — a fixed
// budget here fails as a timeout, which reads like a shift that was never measured.
const ROUTE_BUDGET_MS = 4_000;

async function expectSettled(page: Page, routes: string[]) {
  test.setTimeout(routes.length * ROUTE_BUDGET_MS + 10_000);
  await page.addInitScript(OBSERVER);
  const found: { route: string; shift: Shift }[] = [];
  for (const route of routes) {
    for (const shift of await settleAndCollect(page, route)) found.push({ route, shift });
  }
  expect(found, report(found)).toEqual([]);
}

test("agent states hold still once the app says it is ready", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_AGENT_STATES.map((state) => `fixture=agent&state=${state}`),
  );
});

test("work index states hold still once the app says it is ready", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_WORK_INDEX_STATES.map((state) => `fixture=shell&state=${state}`),
  );
});

test("workspace states hold still once the app says it is ready", async ({ page }) => {
  await expectSettled(
    page,
    VISUAL_WORKSPACE_STATES.map((state) => `fixture=workspace&state=${state}`),
  );
});

test("a jump after the app says it is ready is caught", async ({ page }) => {
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

  expect((await shiftsAfterReady(page)).length).toBeGreaterThan(0);
});
