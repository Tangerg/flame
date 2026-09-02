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
