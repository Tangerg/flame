import { expect, test } from "./test";
import { DOCK_MIN_WIDTH_PX, dockWidthFromRatio, maxDockWidth } from "../src/lib/shellGeometry";
import { dockWidthRow } from "../src/plugins/builtin/shell/kernel/panel/dockWidth";

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

const ROW_WIDTHS = [1920, 1440, 1120, 800, 672, 400];

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

  test("holds the floor on a row too narrow to grant it", async ({ page }) => {
    const narrow = 400;
    expect(maxDockWidth(narrow)).toBe(DOCK_MIN_WIDTH_PX);
    for (const ratio of RATIOS) {
      expect(Math.round(await cssMeasure(page, ratio, narrow))).toBe(DOCK_MIN_WIDTH_PX);
    }
  });
});

const NAMING_VIEWS = [
  "dock-agent-docs",
  "dock-skills",
  "dock-knowledge",
  "dock-search",
  "dock-inbox",
  "dock-review",
  "dock-runs",
  "dock-timeline",
] as const;

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

  expect(examined, "no truncating name was found in any dock view").toBeGreaterThan(20);
  expect(gone, "a name the row exists to identify was squeezed out of the row").toEqual([]);
});
