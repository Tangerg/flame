import { expect, test } from "./test";

// A plan step's mark belongs to the step's FIRST line, not to the middle of however many lines
// it happens to need.
//
// `align-items: center` looks right for as long as every step fits on one line, and every step
// in every fixture does. It was measured wrong the moment one did not: on a two-line step the
// 16px mark sat 10.1px below the centre of the sentence it marks — exactly half the leading —
// so the ring floated between the two lines and read as belonging to neither.
//
// The product already disagreed with itself about this. `ActivePlan`, which renders the same
// steps into the plan pill, had reached `flex-start` on its own, so the same step was laid out
// differently depending on which surface was showing it.
//
// THE GUARD HAS TO MAKE THE CONDITION ITSELF. No fixture has a step long enough to wrap, and
// the dock's width does not follow the viewport, so narrowing the window proves nothing —
// verified across 1472 / 1000 / 820, where all three read one line and zero drift. Writing a
// long string into the label is not measuring the fixture; it is measuring what the shipped
// component's CSS does with content it will certainly meet.
const ROUTE = "/visual/?fixture=workspace&state=dock-light&theme=light";

const LONG =
  "Verify that every boundary owner keeps its invariant when the run is resumed after a " +
  "restart, and that the projection never advances the fact on its own account";

test("a plan step that wraps keeps its mark on the first line", async ({ page }) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto(ROUTE);
  await page.locator("html[data-visual-ready]").waitFor();
  await page.waitForTimeout(300);

  const measured = await page.evaluate((long) => {
    // A `StepRow` by its shape: a flex row whose two children are a 16px-wide grid mark and a
    // span. Found by structure rather than by class, because a StyleX class is a hash.
    const rows: HTMLElement[] = [];
    for (const row of document.querySelectorAll("div")) {
      if (row.closest("[data-fixture-chrome]")) continue;
      const kids = [...row.children];
      if (kids.length !== 2) continue;
      const [mark, label] = kids;
      if (!mark || !label) continue;
      if (mark.tagName !== "DIV" || label.tagName !== "SPAN") continue;
      if (getComputedStyle(mark).display !== "grid") continue;
      if (getComputedStyle(row).display !== "flex") continue;
      if (Math.abs(mark.getBoundingClientRect().width - 16) > 1) continue;
      rows.push(row as HTMLElement);
    }

    const read = (row: HTMLElement) => {
      const mark = row.children[0] as HTMLElement;
      const label = row.children[1] as HTMLElement;
      const m = mark.getBoundingClientRect();
      const l = label.getBoundingClientRect();
      const leading = Number.parseFloat(getComputedStyle(label).lineHeight);
      return {
        lines: Math.max(1, Math.round(l.height / leading)),
        // Positive means the mark sits BELOW the middle of the first line.
        drift: Math.round((m.top + m.height / 2 - (l.top + leading / 2)) * 10) / 10,
      };
    };

    const found = rows.length;
    const short = rows.map(read);
    for (const row of rows) (row.children[1] as HTMLElement).textContent = long;
    const wrapped = rows.map(read);
    return { found, short, wrapped };
  }, LONG);

  // Floors, not targets: a route with no plan steps agrees with any alignment.
  expect(measured.found, "the plan pane has to render steps").toBeGreaterThan(1);
  // And the forced content has to actually wrap, or the interesting case was never reached.
  expect(
    measured.wrapped.filter((one) => one.lines < 2).length,
    "the long label has to wrap, or this measured the easy case twice",
  ).toBe(0);

  expect(
    measured.short.filter((one) => Math.abs(one.drift) > 1),
    "one-line steps whose mark left the line",
  ).toEqual([]);
  expect(
    measured.wrapped.filter((one) => Math.abs(one.drift) > 1),
    "wrapped steps whose mark drifted off the first line",
  ).toEqual([]);
});
