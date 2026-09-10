import { expect, test } from "./test";

// The last appearance preference nothing could see. `fontSmoothing` changes RASTERISATION: it
// moves no box, lands in no computed length, and shifts no golden far enough to fail one — so
// every audit in this suite looked straight past it, and the only test it had asserted that the
// custom property held the string it had just been handed.
//
// That is a plumbing test. It passes just as happily if the property no longer reaches anything,
// which is the failure this preference is actually exposed to: it is two vendor-prefixed
// declarations set as inline style on `<html>`, over an identical pair declared in globals.css.
//
// So this reads the pixels, and asks the two questions a screenshot cannot:
//
//   1. Does the setting change the rendering at all?
//   2. Is `antialiased` the LIGHTER of the two — which is what the word means?
//
// The second is the one that carries meaning. A wired-up-but-inverted boolean passes the first
// question and fails this one, and inverting it is the single most likely way for this to break,
// because nothing else in the product would notice.
//
// Measured when written: 4.65% of the sidebar's pixels differ, mean delta 16.8/255, and `auto`
// lays down 5.7% more ink than `antialiased`.
//
// Scope, stated rather than implied: this proves the preference reaches rasterisation in the
// browser the audit runs in. The desktop app ships in a WKWebView, whose font rendering is its
// own; the same limitation DESIGN.md records for `corner-shape` applies here. What is portable
// is the wiring and the direction, and those are what this holds.
// Named, and checked to be the one thing it names. `aside` alone is positional: it happens to
// match only the product's drawer today, and the agent fixture already carries a scaffold
// sidebar of its own — so the day the shell fixture grows one, this would photograph the
// harness twice and still pass, because it only compares the region against itself. The
// count assertion below is what turns that into a red test instead of a quiet one.
const REGION = "aside.agent-drawer";

/** How much of the region differs, and how much ink each setting lays down. */
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

  // Decoded in the page, because a PNG buffer in Node has nothing to decode it with and the
  // page already has the one decoder that is guaranteed to agree with what was photographed.
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
        // Ink is distance from the light scheme's paper. Summed over the region it is a proxy for
        // how heavy the text is set, which is the one thing the two modes disagree about.
        inkAntialiased += 255 - lumaOne;
        inkAuto += 255 - lumaTwo;
        const apart = Math.abs(lumaOne - lumaTwo);
        // Above the noise floor of a deterministic renderer, which is zero — the margin is for
        // the PNG round trip, not for the rasteriser.
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
