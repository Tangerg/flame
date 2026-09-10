import { expect, test, type Page } from "./test";

// The transcript follows a growing turn only while the reader is at the bottom of it.
//
// This was a recorded HARNESS GAP rather than a doubt about the product: nothing in the fixture
// set streams, so the one behaviour that only exists while content is arriving had no way to be
// exercised, and three separate rounds noted it and moved on.
//
// It does not need a streaming fixture. `use-stick-to-bottom` reacts to the CONTENT box getting
// taller, so a test can make that happen directly — the same move as forcing a plan step to
// wrap. Appending a block to the content element is not measuring the fixture; it is measuring
// what the shipped scroller does when the thing it is watching grows, which is the whole
// contract.
//
// Three halves, because two of them look identical from the wrong end: a scroller that always
// jumps to the bottom passes "it followed", and one that never scrolls passes "it did not
// jump". Only asserting both, plus the way back, pins the behaviour.
const ROUTE = "/visual/?fixture=agent&state=long-content&theme=light";
const JUMP = "Jump to bottom";

/** How much taller each append makes the transcript. Larger than any threshold the library
 *  could reasonably use, so "it moved" and "it did not" are never a rounding argument. */
const GROWTH_PX = 400;

async function land(page: Page) {
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.locator(".msg-scroll-viewport").waitFor();
  await page.waitForTimeout(500);
}

/** Appends a tall block to the transcript's content element, the way a streamed turn grows it. */
async function grow(page: Page): Promise<boolean> {
  return page.evaluate((height) => {
    const content = document.querySelector(".msg-scroll-viewport")?.firstElementChild;
    if (!content) return false;
    const block = document.createElement("div");
    block.style.height = `${height}px`;
    block.dataset.grownByTest = "";
    content.appendChild(block);
    return true;
  }, GROWTH_PX);
}

async function read(page: Page) {
  const reading = await page.evaluate(() => {
    const v = document.querySelector(".msg-scroll-viewport") as HTMLElement | null;
    if (!v) return null;
    return {
      top: Math.round(v.scrollTop),
      fromBottom: Math.round(v.scrollHeight - v.clientHeight - v.scrollTop),
      scrollable: v.scrollHeight - v.clientHeight,
    };
  });
  expect(reading, "the transcript viewport has to be on the page").not.toBeNull();
  return reading!;
}

test("a growing transcript follows the tail, but only from the bottom", async ({ page }) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1120, height: 720 });

  // --- At the bottom: the tail stays in view.
  await land(page);
  const room = (await read(page)).scrollable;
  // Floor, not a target: a transcript that fits its viewport can neither follow nor fail to.
  expect(room, "the route has to render more transcript than fits").toBeGreaterThan(GROWTH_PX * 2);

  await page.locator(".msg-scroll-viewport").hover();
  await page.mouse.wheel(0, room + 1000);
  await page.waitForTimeout(600);
  expect((await read(page)).fromBottom, "the wheel has to reach the bottom").toBeLessThanOrEqual(2);
  await expect(page.getByRole("button", { name: JUMP })).toHaveAttribute("tabindex", "-1");

  expect(await grow(page), "the append has to find the content element").toBe(true);
  await page.waitForTimeout(700);
  const followed = await read(page);
  expect(followed.fromBottom, "content arrived and the tail left the viewport").toBeLessThanOrEqual(
    2,
  );

  // --- Scrolled up: the reader's place is theirs.
  await land(page);
  await page.locator(".msg-scroll-viewport").hover();
  await page.mouse.wheel(0, room + 1000);
  await page.waitForTimeout(500);
  await page.mouse.wheel(0, -600);
  await page.waitForTimeout(600);
  const parked = await read(page);
  expect(parked.fromBottom, "the reader has to actually be away from the bottom").toBeGreaterThan(
    100,
  );

  expect(await grow(page), "the append has to find the content element").toBe(true);
  await page.waitForTimeout(700);
  const held = await read(page);
  expect(held.top, "content arrived and the view moved under the reader").toBe(parked.top);
  expect(
    held.fromBottom - parked.fromBottom,
    "the new material has to be below the reader, not around them",
  ).toBe(GROWTH_PX);

  // --- And the way back is offered, and works.
  //
  // Polled, not waited: the jump is ANIMATED. Measured from a 600px offset it reads 184px away
  // after 200ms, 47px after 400ms, 3px after 800ms and settles at 1px — so a fixed wait either
  // catches it in flight (this asserted 4px at 700ms and read 12) or has to be long enough to
  // be a guess. What the contract says is where it ARRIVES, not how fast.
  const jump = page.getByRole("button", { name: JUMP });
  await expect(jump).toHaveAttribute("tabindex", "0");
  await jump.click();
  await expect
    .poll(async () => (await read(page)).fromBottom, { timeout: 5_000 })
    .toBeLessThanOrEqual(2);
});
