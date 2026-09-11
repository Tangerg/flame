import { expect, test } from "./test";

// When the composer's toolbar runs out of room, the chips give up their LABELS — they do not
// ellipse them.
//
// `useToolbarLabels` says why that cannot be CSS: flexbox shrinks every chip at once and each
// label box is sized to its own text, so the row arrives at three two-character stubs
// (`Balanc…`, `GP…`, `Mediu…`) instead of at one row that dropped its words. The mechanism is a
// measurement — natural width read with `data-measuring` on for a single reflow — and it rests
// on an `!important` that has to outrank the chips' own StyleX `flex-shrink`.
//
// None of that is exercised by any fixture. Measured across 1472 → 560: the footer is 768px
// wide and its natural width is 768px at every one of them, so `labelled` never flips and a
// regression would be invisible. The condition has to be made — the footer is what the
// ResizeObserver watches, so giving it a width IS the reader's narrow window as far as the
// mechanism is concerned.
const ROUTE = "/visual/?fixture=agent&state=idle&theme=light";

/** Narrow enough that the labels cannot fit, wide enough that the chips still can. */
const SQUEEZED_PX = 340;

test("a toolbar with no room drops its labels rather than ellipsing them", async ({ page }) => {
  await page.setViewportSize({ width: 1120, height: 800 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.locator(".agent-composer-footer").waitFor();
  await page.waitForTimeout(400);

  const read = () =>
    page.evaluate(() => {
      const footer = document.querySelector(".agent-composer-footer") as HTMLElement | null;
      if (!footer) return null;
      const labels = [...footer.querySelectorAll('[data-slot="composer-chip-label"]')];
      // The same reading `measure()` takes: natural width is only knowable with the labels
      // un-hidden, which is the whole reason that attribute exists.
      footer.dataset.measuring = "";
      const natural = footer.scrollWidth;
      delete footer.dataset.measuring;
      return {
        labelled: footer.hasAttribute("data-labelled"),
        shown: labels.filter((label) => (label as HTMLElement).offsetParent !== null).length,
        natural,
        available: footer.clientWidth,
        overflow: footer.scrollWidth - footer.clientWidth,
        ellipsed: [...footer.querySelectorAll("*")]
          .filter((node) => {
            const style = getComputedStyle(node);
            return node.scrollWidth - node.clientWidth > 1 && style.textOverflow === "ellipsis";
          })
          .map((node) => (node.textContent ?? "").trim().slice(0, 24)),
      };
    });

  const roomy = await read();
  expect(roomy, "the composer has to render its toolbar").not.toBeNull();
  // Floors: a toolbar with no labels, or one already too narrow, tests nothing below.
  expect(roomy!.shown, "the toolbar has to have labels to give up").toBeGreaterThan(1);
  expect(roomy!.labelled).toBe(true);
  expect(roomy!.ellipsed, "nothing should be ellipsed while there is room").toEqual([]);

  await page.evaluate((width) => {
    const footer = document.querySelector(".agent-composer-footer") as HTMLElement | null;
    if (footer) footer.style.maxWidth = `${width}px`;
  }, SQUEEZED_PX);
  await page.waitForTimeout(400);

  const squeezed = await read();
  // The squeeze has to actually squeeze — and this is also where the mechanism fails first.
  // Verified by deleting the `!important` on `[data-measuring] > *`: the natural width comes
  // back as the squeezed width itself, because the chips shrank while being measured. That is
  // the comment's own sentence — "it makes the row measure as fitting and the labels never go"
  // — so a reading of exactly `SQUEEZED_PX` means the row cannot see its own size, not that
  // this test asked too little.
  expect(
    squeezed!.natural,
    `the labelled row measured ${squeezed!.natural}px inside ${SQUEEZED_PX}px — either the ` +
      `squeeze is too wide, or the chips are shrinking while being measured`,
  ).toBeGreaterThan(SQUEEZED_PX);

  expect(squeezed!.labelled, "the toolbar kept claiming it had room").toBe(false);
  expect(squeezed!.shown, "labels that stayed to be ellipsed instead of dropping").toBe(0);
  expect(squeezed!.ellipsed, "chips ellipsed rather than the row dropping its labels").toEqual([]);
  expect(squeezed!.overflow, "the row still does not fit once the labels are gone").toBeLessThan(2);
});
