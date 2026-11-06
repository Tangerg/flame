import { test } from "./test";
import { keepFinalSortRevealed, respectHorizontalReading } from "./genuiFocus";

test.use({ browserName: "webkit" });

for (const theme of ["light", "dark"] as const) {
  test(`WebKit keeps the focused final sort control revealed in ${theme}`, async ({ page }) => {
    await keepFinalSortRevealed(page, theme);
  });
  test(`WebKit respects manual horizontal reading in ${theme}`, async ({ page }) => {
    await respectHorizontalReading(page, theme);
  });
}
