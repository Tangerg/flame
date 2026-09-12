import { expect, test } from "./test";

const SCALES = [0.6, 1, 1.4] as const;

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=agent&state=narrative",
  "fixture=foundation",
] as const;

type Corners = { own: number[]; outers: number[][] };

async function read(
  page: import("@playwright/test").Page,
  url: string,
): Promise<Record<string, Corners>> {
  await page.goto(url);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(300);
  return page.evaluate(() => {
    const path = (element: Element): string => {
      const parts: string[] = [];
      let node: Element | null = element;
      while (node && node !== document.documentElement) {
        const parent: Element | null = node.parentElement;
        parts.push(
          `${node.tagName.toLowerCase()}:${parent ? [...parent.children].indexOf(node) : 0}`,
        );
        node = parent;
      }
      return parts.reverse().join(">");
    };

    const radii = (element: Element): number[] | null => {
      const style = getComputedStyle(element);
      const corners = [
        style.borderTopLeftRadius,
        style.borderTopRightRadius,
        style.borderBottomRightRadius,
        style.borderBottomLeftRadius,
      ];
      if (corners.some((corner) => corner.endsWith("%"))) return null;
      const values = corners.map((corner) => Number.parseFloat(corner));
      if (values.some((value) => !Number.isFinite(value) || value >= 1000)) return null;
      if (values.every((value) => value === 0)) return null;
      return values;
    };

    const out: Record<string, Corners> = {};
    for (const element of document.querySelectorAll("*")) {
      if (element.closest("[data-fixture-chrome]")) continue;
      const box = element.getBoundingClientRect();
      if (box.width < 2 || box.height < 2) continue;
      const own = radii(element);
      if (own === null) continue;

      const outers: number[][] = [];
      for (let parent = element.parentElement; parent; parent = parent.parentElement) {
        const found = radii(parent);
        if (found !== null) outers.push(found);
      }
      out[path(element)] = { own, outers };
    }
    return out;
  });
}

test("every corner rides the one ladder the radius preference multiplies", async ({ page }) => {
  test.setTimeout(ROUTES.length * SCALES.length * 20_000 + 30_000);
  await page.setViewportSize({ width: 1472, height: 900 });

  const stranded: string[] = [];
  let compared = 0;

  for (const route of ROUTES) {
    const readings = new Map<number, Record<string, Corners>>();
    for (const scale of SCALES) {
      readings.set(scale, await read(page, `/visual/?${route}&theme=light&radius=${scale}`));
    }
    const base = readings.get(1)!;

    for (const [key, at1] of Object.entries(base)) {
      for (const scale of SCALES) {
        if (scale === 1) continue;
        const atK = readings.get(scale)![key];
        if (atK === undefined) continue;

        for (let corner = 0; corner < 4; corner += 1) {
          const from = at1.own[corner]!;
          const to = atK.own[corner]!;
          if (from === 0 && to === 0) continue;
          compared += 1;

          if (Math.abs(to - from * scale) <= 0.2) continue;
          const concentric = at1.outers.some((outerAt1, depth) => {
            const outerAtK = atK.outers[depth];
            if (outerAtK === undefined) return false;
            const outerFrom = outerAt1[corner]!;
            const outerTo = outerAtK[corner]!;
            if (outerFrom <= 0 || outerFrom < from - 0.2) return false;
            return (
              Math.abs(outerTo - outerFrom * scale) <= 0.2 &&
              Math.abs(outerTo - to - (outerFrom - from)) <= 0.2
            );
          });
          if (concentric) continue;
          stranded.push(
            `${route} @${scale}  ${key}  corner ${corner}: ${from}px → ${to}px` +
              ` (inside ${at1.outers.map((one, depth) => `${one[corner]}→${atK.outers[depth]?.[corner]}`).join(", ") || "nothing rounded"})`,
          );
        }
      }
    }
  }

  expect(compared, "the sweep has to reach real corners").toBeGreaterThan(1000);
  expect(
    [...new Set(stranded)].slice(0, 40),
    "corners that are neither a rung nor a setback from one",
  ).toEqual([]);
});

test("the segmented chip stays concentric inside its track at every radius", async ({ page }) => {
  test.setTimeout(SCALES.length * 20_000 + 20_000);
  await page.setViewportSize({ width: 1472, height: 900 });

  const drift: string[] = [];
  for (const scale of SCALES) {
    await page.goto(`/visual/?fixture=workspace&state=settings&theme=light&radius=${scale}`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(300);

    const measured = await page
      .locator('[data-orientation="horizontal"]')
      .first()
      .evaluate((track) => {
        const trackBox = track.getBoundingClientRect();
        const trackRadius = Number.parseFloat(getComputedStyle(track).borderTopLeftRadius);
        const chip = [...track.querySelectorAll("*")].find((node) => {
          const style = getComputedStyle(node);
          return (
            style.backgroundColor !== "rgba(0, 0, 0, 0)" && node.getBoundingClientRect().height > 8
          );
        });
        if (!chip) return null;
        const chipBox = chip.getBoundingClientRect();
        const chipRadius = Number.parseFloat(getComputedStyle(chip).borderTopLeftRadius);
        const setback = chipBox.top - trackBox.top;
        return { trackRadius, chipRadius, setback, error: chipRadius - (trackRadius - setback) };
      });

    expect(measured, "the settings route has to render a segmented control").not.toBeNull();
    const { trackRadius, chipRadius, setback, error } = measured!;
    if (Math.abs(error) > 0.2) {
      drift.push(
        `@${scale}: track ${trackRadius}px, chip ${chipRadius}px, setback ${setback}px — off concentric by ${error.toFixed(2)}px`,
      );
    }
  }

  expect(drift, "the chip's setback from its track has to hold across the ladder").toEqual([]);
});
