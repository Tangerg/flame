import { expect, test } from "./test";
import { DOCK_MIN_WIDTH_PX, dockWidthFromRatio, maxDockWidth } from "../src/lib/shellGeometry";
import { dockWidthRow } from "../src/plugins/builtin/shell/kernel/panel/dockWidth";

// The dock's range is stated TWICE and has to be — the CSS copy is what lets a window
// resize re-derive the measure with no React render, and the TS copy is what clamps a
// live drag and converts the settled width back to the stored ratio. Neither can read
// the other: TS runs before layout, and CSS cannot call a function.
//
// So the duplication stays and the AGREEMENT gets the guard. Nothing else checks it:
// `shellGeometry.test.ts` exercises the TS side against itself, and a divergence would
// surface only as a flank that stops following the pointer near the ends of its travel
// — which reads as a rendering quirk, not as two formulas that no longer match.
//
// The probe rebuilds the real consumption exactly: a flex row of a known width carrying
// the row style, and a child taking `flex: 0 0 var(--dock-measure)` the way
// `.agent-context-dock` does. Percentages inside the measure resolve against the row,
// which is both the flex container and the containing block.

async function cssMeasure(
  page: import("@playwright/test").Page,
  ratio: number,
  rowWidth: number,
): Promise<number> {
  return page.evaluate(
    ({ style, width }) => {
      const row = document.createElement("div");
      row.style.cssText = "display:flex;position:absolute;left:-99999px;top:0;";
      row.style.width = `${width}px`;
      for (const [property, value] of Object.entries(style)) {
        row.style.setProperty(property, String(value));
      }
      const flank = document.createElement("div");
      flank.style.flex = "0 0 var(--dock-measure)";
      row.append(flank);
      document.body.append(row);
      const measured = flank.getBoundingClientRect().width;
      row.remove();
      return measured;
    },
    { style: dockWidthRow(ratio) as Record<string, string>, width: rowWidth },
  );
}

const ROW_WIDTHS = [
  1920, // a display wide enough that the preferred measure is never the binding claim
  1440,
  1120, // the suite's own viewport
  800,
  672, // exactly floor + safe area: the narrowest row that can present both
  400, // too narrow for the floor — the range collapses and must not invert
];

const RATIOS = [0, 0.25, 0.5, 0.75, 1];

test.describe("the dock measure agrees between TypeScript and CSS", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/visual/?fixture=foundation&theme=light");
    await page.waitForSelector("html[data-visual-ready]");
  });

  test("resolves to the width the drag arithmetic assumes, across the range", async ({ page }) => {
    for (const rowWidth of ROW_WIDTHS) {
      for (const ratio of RATIOS) {
        const measured = await cssMeasure(page, ratio, rowWidth);
        const expected = dockWidthFromRatio(ratio, rowWidth);
        // The TS side rounds to whole pixels and CSS does not, so they may differ by
        // the rounding and by nothing else.
        expect(
          Math.abs(measured - expected),
          `row ${rowWidth}px at ratio ${ratio}: CSS painted ${measured}, drag assumed ${expected}`,
        ).toBeLessThan(1);
      }
    }
  });

  test("puts the ends of the range exactly where the clamp does", async ({ page }) => {
    for (const rowWidth of ROW_WIDTHS) {
      expect(Math.round(await cssMeasure(page, 0, rowWidth))).toBe(DOCK_MIN_WIDTH_PX);
      expect(Math.round(await cssMeasure(page, 1, rowWidth))).toBe(maxDockWidth(rowWidth));
    }
  });

  // The floor has one owner on each side — `Math.max` in the module, `max()` in the
  // measure — and a row too narrow to grant it is the only place either shows. Lose the
  // CSS one and this row paints a flank of 48px that no drag can reach.
  test("holds the floor on a row too narrow to grant it", async ({ page }) => {
    const narrow = 400;
    expect(maxDockWidth(narrow)).toBe(DOCK_MIN_WIDTH_PX);
    for (const ratio of RATIOS) {
      expect(Math.round(await cssMeasure(page, ratio, narrow))).toBe(DOCK_MIN_WIDTH_PX);
    }
  });
});

// What the dock's floor costs the ROWS inside it.
//
// The range above is about the flank; this is about what a view has to fit into once the
// flank is at the narrow end of it. A row that names something — a skill, a proposal, a
// recipe — puts that name beside chips that keep their width, and the name is the only
// part with `text-overflow: ellipsis`, which gives a flex item an automatic minimum of
// ZERO. So the one thing identifying the row is the only thing in it allowed to vanish:
// measured in a 207px title column, `review-diff` rendered at 0px wide beside a revision
// hash and two badges that were all fully drawn. Not ellipsed — absent.
//
// No golden could see it. `workspace.visual.spec.ts` photographs the dock at the 1472px
// canonical viewport, where the same column is 560px and every name fits.
//
// `checkVisibility` is load-bearing: a view that is mounted but not shown reports a 0px
// box for everything in it, and without asking the browser whether the element is
// actually rendered this reads 200 of those and none of the real one.
const NAMING_VIEWS = [
  "dock-skill-proposals",
  "dock-skill-library",
  "dock-recipes",
  "dock-agent-docs",
  "dock-skills",
  "dock-knowledge",
  "dock-search",
  "dock-files",
  "dock-inbox",
  "dock-review",
  "dock-runs",
  "dock-timeline",
] as const;

/** Under this a box shows no glyph and no ellipsis either — the text is simply not there. */
const LEGIBLE_PX = 12;

test("a dock near its floor never renders a name at zero width", async ({ page }) => {
  test.setTimeout(180_000);
  await page.setViewportSize({ width: 1120, height: 820 });

  const gone: string[] = [];
  let examined = 0;

  for (const state of NAMING_VIEWS) {
    await page.goto(`/visual/?fixture=workspace&state=${state}&theme=light`);
    await page.locator("html[data-visual-ready]").waitFor();

    const seen = await page.evaluate((legible) => {
      const out = { examined: 0, gone: [] as string[] };
      for (const element of document.querySelectorAll<HTMLElement>("main *")) {
        if (getComputedStyle(element).textOverflow !== "ellipsis") continue;
        if (!element.checkVisibility({ visibilityProperty: true, contentVisibilityAuto: true })) {
          continue;
        }
        const text = [...element.childNodes]
          .filter((node) => node.nodeType === Node.TEXT_NODE)
          .map((node) => node.textContent ?? "")
          .join("")
          .trim();
        if (text.length < 3) continue;
        out.examined += 1;
        const width = element.getBoundingClientRect().width;
        if (width < legible) out.gone.push(`${Math.round(width)}px "${text.slice(0, 32)}"`);
      }
      return out;
    }, LEGIBLE_PX);

    examined += seen.examined;
    for (const hit of seen.gone) gone.push(`${state}: ${hit}`);
  }

  // Floor: these views name things, so a run that found nothing to measure is a run whose
  // selector stopped matching rather than a dock that got wider.
  expect(examined, "no truncating name was found in any dock view").toBeGreaterThan(20);
  expect(gone, "a name the row exists to identify was squeezed out of the row").toEqual([]);
});
