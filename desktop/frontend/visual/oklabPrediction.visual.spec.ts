import { expect, test } from "./test";
import { mixOklab } from "@/plugins/builtin/theme/kit/legibility";

const PAIRS = [
  ["#000000", "#ffffff"],
  ["#e3e5e9", "#1d1f23"],
  ["#2b2a26", "#f5f0e8"],
  ["#3574f0", "#ffe066"],
  ["#30a46c", "#202020"],
] as const;

const PERCENTAGES = [0, 12, 34, 53, 71, 88, 100] as const;

test("the predicted oklab mix is the one the browser paints", async ({ page }) => {
  await page.goto("/visual/?fixture=agent&state=narrative&theme=light");
  await page.locator("html[data-visual-ready]").waitFor();

  const cases = PAIRS.flatMap(([ink, fill]) =>
    PERCENTAGES.map((pct) => ({ ink, fill, pct, predicted: mixOklab(ink, fill, pct) })),
  );

  const painted = await page.evaluate(
    (cases) =>
      cases.map(({ ink, fill, pct }) => {
        const canvas = document.createElement("canvas");
        canvas.width = canvas.height = 1;
        const context = canvas.getContext("2d")!;
        context.fillStyle = `color-mix(in oklab, ${ink} ${pct}%, ${fill})`;
        context.fillRect(0, 0, 1, 1);
        const [r, g, b] = context.getImageData(0, 0, 1, 1).data;
        return `#${[r, g, b].map((v) => (v ?? 0).toString(16).padStart(2, "0")).join("")}`;
      }),
    cases,
  );

  const drift: string[] = [];
  let compared = 0;
  cases.forEach((one, index) => {
    compared += 1;
    const actual = painted[index]!;
    const apart = [1, 3, 5].map((at) =>
      Math.abs(
        Number.parseInt(actual.slice(at, at + 2), 16) -
          Number.parseInt(one.predicted.slice(at, at + 2), 16),
      ),
    );
    if (Math.max(...apart) > 1) {
      drift.push(
        `${one.ink} ${one.pct}% over ${one.fill}: painted ${actual}, predicted ${one.predicted}`,
      );
    }
  });

  expect(compared, "the browser has to have painted something comparable").toBeGreaterThan(20);
  expect(
    drift,
    "the prediction the ink ladder is chosen from has drifted from the browser",
  ).toEqual([]);
});
