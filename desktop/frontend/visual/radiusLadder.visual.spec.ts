import { expect, test } from "./test";

// `radiusScale` was the last appearance preference nothing measured. DESIGN.md states it as a
// promise about the product rather than about the tokens — "the visual style owns the ladder;
// the user's radius preference multiplies through", and, under NEVER, "no mixed scales on one
// screen, no step invented at a call site". A token test cannot see either one: it reads the
// same `calc()` the ladder is written in and agrees with itself. Only corners can answer.
//
// So this reads every corner the product actually paints, at each of the three settings the
// picker offers. A corner is allowed to be one of exactly two things, and both are the ladder
// reaching it:
//
//   PROPORTIONAL — it is a rung, so it moves by the factor.
//   CONCENTRIC   — it sits inside another corner, so it keeps a constant setback from it and
//                  the PAIR moves by the factor. `--segment-radius` and
//                  `--composer-attachment-radius` are both this: a chip inside a track, an
//                  attachment inside the composer.
//
// Anything else is a corner that never joined the ladder — a literal at a call site, a
// hard-coded step, or a rung someone unhooked from the scale.
//
// The second branch is stated as an invariant rather than a list of tokens on purpose. The
// first version of this audit knew only about proportionality and carried the one concentric
// radius as a named exception; the next concentric radius added to the product failed it, which
// is a guard that has to be edited every time the design does the right thing.
//
// Measured when written: 1660 corners across five routes, all three scales, none off either
// branch. Unhooking one rung (`--shape-lg` from `--radius-scale`) strands 680 of them; freezing
// a concentric one (`segment-radius` at a literal) strands 200.
//
// What it deliberately does NOT catch: a concentric pair spelled as two independent rungs. Both
// are proportional, so both pass — which is right, because this audit asks whether the ladder
// reaches a corner, not whether the corner sits correctly inside its neighbour. The test below
// asks that.
const SCALES = [0.6, 1, 1.4] as const;

const ROUTES = [
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=agent&state=narrative",
  "fixture=foundation",
] as const;

/**
 * Per element: its own four corners, and those of every rounded corner it sits inside, nearest
 * first.
 *
 * Every ancestor rather than the nearest one, because setbacks compose. A segmented control is
 * three deep — track, tab, chip — and the tab is already a setback from the track, so measuring
 * the chip against its nearest rounded ancestor measures it against another setback and finds
 * nothing proportional to stand on.
 */
type Corners = { own: number[]; outers: number[][] };

async function read(
  page: import("@playwright/test").Page,
  url: string,
): Promise<Record<string, Corners>> {
  await page.goto(url);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(300);
  return page.evaluate(() => {
    // Identity across three page loads. The routes render deterministically, so the position
    // chain is stable; a key built from tag and class would collapse the dozens of rows that
    // share both, which is exactly the group a wrong radius shows up in.
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

    // A percentage is relative to the box and travels with it, not with the ladder; `pill` is a
    // cap rather than a rung, and 9999px times anything is still a lozenge. Both read as "not a
    // number this audit can reason about".
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
          // Concentric: some corner it sits inside IS a rung, and the setback from that corner
          // is the same at both settings — so the pair moved together.
          const concentric = at1.outers.some((outerAt1, depth) => {
            const outerAtK = atK.outers[depth];
            if (outerAtK === undefined) return false;
            const outerFrom = outerAt1[corner]!;
            const outerTo = outerAtK[corner]!;
            // A square corner is not a rung, and a "setback" from one is not concentric — it is
            // the arithmetic saying nothing. Verified: without these two, freezing
            // `--segment-radius` at a literal still passed, because a 0px ancestor corner
            // scales to 0px and leaves a setback that is constant at minus the child's own
            // radius. A real setback also means the child sits INSIDE what it is measured
            // against, so it can never be the rounder of the two.
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

  // Floor, not a target: an audit that reached no corners agrees with every scale.
  expect(compared, "the sweep has to reach real corners").toBeGreaterThan(1000);
  expect(
    [...new Set(stranded)].slice(0, 40),
    "corners that are neither a rung nor a setback from one",
  ).toEqual([]);
});

// The one nesting in the product where a corner sits directly inside another with only a border
// and a padding between them: the segmented control's moving chip inside its track. DESIGN.md's
// polish rules ask nested corners to stay visually concentric, which for a scaling ladder means
// the SETBACK is what has to hold still — not the radius.
//
// It did not. The track is on `md` and the chip was on `sm`, and `--corner-scale` applies at
// `md` and above but not below, so the pair's gap grew with the preference: measured 2.4px /
// 4.0px / 5.6px at sharp / default / soft where the geometry asks for a constant 3px, and the
// chip read progressively squarer inside its own track. Deriving the chip from the track makes
// the error 0.00 at all three.
//
// Verified to fail: spelling the chip back as its own rung (`var(--shape-sm)`) reports
// +0.60 / -1.00 / -2.60 — too round at sharp, too square at soft, and only close at the one
// setting the pair was eyeballed at.
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
        // The chip is the one painted descendant; the tabs themselves are transparent at rest.
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
