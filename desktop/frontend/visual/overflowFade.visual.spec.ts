import { expect, test } from "./test";

// A label whose text does not fit is clipped by `overflow: hidden`, so the only question is
// whether the cut is soft. It has to be soft in BOTH states, and the soft edge has to be the
// one the text is actually crossing:
//
//   at rest      the text runs off the right, so the right edge fades
//   while moving the head is leaving on the left, and the tail is landing flush on the right,
//                so the left edge fades and the right edge stays crisp
//
// The failure this exists for is not a missing fade but a REMOVED one: the hover rule used to
// say `mask-image: none`, which reveals nothing at all — the box still clips — and so traded a
// soft edge for a hard cut at exactly the moment text was travelling through it. Measured on
// the work index: three overflowing labels, hard-clipped on both sides for the whole reveal.
//
// Asserted as "which edge is transparent" rather than as the gradient's text, so the fade
// distance can change without touching this and a swapped edge cannot.
const ROUTE = "/visual/?fixture=shell&state=populated&theme=light";

const OPAQUE = "rgb(0, 0, 0)";
const CLEAR = "rgba(0, 0, 0, 0)";

/** Which end of the gradient is the transparent one, by stop order in the computed value. */
function fadedEdge(mask: string): "left" | "right" | "neither" {
  const clear = mask.indexOf(CLEAR);
  const opaque = mask.indexOf(OPAQUE);
  if (clear < 0 || opaque < 0) return "neither";
  return clear < opaque ? "left" : "right";
}

test("an overflowing label keeps a soft edge, on the side the text is crossing", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 720 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(400);

  const labels = page.locator(".agent-overflow-label[data-overflowing]");
  const count = await labels.count();
  // Floor, not a target: a route where nothing overflows agrees with any mask at all. The
  // narrow viewport is what produces the condition — at 1472 the work index has room.
  expect(count, "the route has to render labels that actually overflow").toBeGreaterThan(0);

  const wrong: string[] = [];
  for (let index = 0; index < count; index += 1) {
    const label = labels.nth(index);
    const rest = await label.evaluate((node) => ({
      mask: getComputedStyle(node).maskImage,
      overflow: getComputedStyle(node).overflow,
      text: (node.textContent ?? "").trim().slice(0, 24),
    }));

    // The premise. If the box ever stops clipping, "soft edge" is the wrong question and this
    // audit should be rewritten rather than kept passing.
    if (rest.overflow !== "hidden") {
      wrong.push(`"${rest.text}" is not clipped (overflow: ${rest.overflow})`);
      continue;
    }
    if (fadedEdge(rest.mask) !== "right") {
      wrong.push(`"${rest.text}" at rest fades ${fadedEdge(rest.mask)}, want right`);
    }

    await label.hover();
    await page.waitForTimeout(150);
    const moving = await label.evaluate((node) => getComputedStyle(node).maskImage);
    if (fadedEdge(moving) !== "left") {
      wrong.push(
        `"${rest.text}" while revealing fades ${fadedEdge(moving)}, want left` +
          (moving === "none" ? " (mask removed — the box still clips)" : ""),
      );
    }
  }

  expect(wrong, "labels whose soft edge is on the wrong side, or gone").toEqual([]);
});
