import { expect, test } from "./test";

const REGION = "aside.agent-drawer";

async function compare(page: import("@playwright/test").Page, route: string) {
  const shots: Record<string, string> = {};
  for (const setting of ["on", "off"]) {
    await page.goto(`/visual/?${route}&theme=light&smoothing=${setting}`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(400);
    await expect(
      page.locator(REGION),
      "the region has to be the product's drawer, and only it",
    ).toHaveCount(1);
    shots[setting] = (await page.locator(REGION).screenshot()).toString("base64");
  }

  return page.evaluate(
    async ([antialiased, auto]) => {
      const pixels = async (encoded: string) => {
        const image = new Image();
        image.src = `data:image/png;base64,${encoded}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = image.naturalWidth;
        canvas.height = image.naturalHeight;
        const context = canvas.getContext("2d")!;
        context.drawImage(image, 0, 0);
        return context.getImageData(0, 0, canvas.width, canvas.height).data;
      };
      const [one, two] = [await pixels(antialiased!), await pixels(auto!)];

      let differing = 0;
      let delta = 0;
      let inkAntialiased = 0;
      let inkAuto = 0;
      for (let at = 0; at < one.length; at += 4) {
        const lumaOne = (one[at]! + one[at + 1]! + one[at + 2]!) / 3;
        const lumaTwo = (two[at]! + two[at + 1]! + two[at + 2]!) / 3;
        inkAntialiased += 255 - lumaOne;
        inkAuto += 255 - lumaTwo;
        const apart = Math.abs(lumaOne - lumaTwo);
        if (apart > 2) {
          differing += 1;
          delta += apart;
        }
      }
      const total = one.length / 4;
      return {
        share: differing / total,
        meanDelta: differing ? delta / differing : 0,
        heavier: inkAuto / inkAntialiased - 1,
      };
    },
    [shots.on, shots.off],
  );
}

test("the font smoothing preference reaches the pixels, and antialiased is the lighter one", async ({
  page,
}) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1472, height: 900 });

  const measured = await compare(page, "fixture=shell&state=populated");

  expect(
    measured.share,
    `the setting changed ${(measured.share * 100).toFixed(2)}% of the region — it is not reaching the rasteriser`,
  ).toBeGreaterThan(0.01);
  expect(measured.meanDelta, "the pixels that changed have to actually differ").toBeGreaterThan(4);
  expect(
    measured.heavier,
    `\`auto\` set text ${(measured.heavier * 100).toFixed(1)}% heavier than \`antialiased\` — antialiased has to be the lighter of the two`,
  ).toBeGreaterThan(0.02);
});
