import { expect, test } from "./test";
import { CONTROL } from "./controls";

// What "the pointer is over this" looks like, read off the screen rather than out of the source.
//
// A static guard already keeps the hover VALUE in one place, and it reads source: it can see
// `":hover": surface.hover` and cannot see what that composites to. The focus ring had just
// shown those two things coming apart completely, so this hovers every control in the fixtures
// and diffs the computed fill.
//
// It found the mechanism contradicting its own model. The design calls these states an ink
// WASH — ink laid over what is there — but they were written into `background-color`, which
// holds one value, so the wash REPLACED the resting fill. Hovering a selected row took it from
// 4% ink to 3%: the pointer made the selection fainter. Hovering a sunken row dropped its
// recess for a translucent neutral, so a well read as if it had popped out of the surface.
// Neither is visible in a screenshot of a resting page, and no golden hovers anything.
//
// Two assertions. The first is the defect's exact shape: a control that HAS a resting fill may
// not land on the plain neutral wash, because that is the wash having replaced it. The second
// is the shape of the answer as a whole — one gesture should have a handful of answers, not one
// per call site, which is the inconsistency the value guard exists for and cannot see.

const ROUTES = [
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

// Hovering is a walk with a settle after every step, so the budget comes from its own work
// rather than Playwright's default — the same reason the chrome-focus walk sets its own.
const STEP_BUDGET_MS = 400;
const CONTROLS_PER_ROUTE = 60;

// One gesture, a small closed set of answers: ink over nothing, ink over a resting fill, a
// filled control stepping its own fill. A ceiling rather than an exact list, because a new tone
// is a legitimate addition and a per-call-site alpha is not — this catches the drift, not the
// growth.
const MAX_DISTINCT_ANSWERS = 8;

test("hover always adds ink, and never replaces the fill it lands on", async ({ page }) => {
  test.setTimeout(ROUTES.length * CONTROLS_PER_ROUTE * STEP_BUDGET_MS + 20_000);
  const answers = new Map<string, string>();
  const replaced: string[] = [];
  const unreachable: string[] = [];
  let hovered = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(250);

    // The neutral wash as the browser resolves it, so the comparison is against the token
    // rather than against a colour written down here that the theme could move.
    const neutralWash = await page.evaluate(() => {
      const probe = document.createElement("div");
      probe.style.backgroundColor = "var(--wash-hover)";
      document.body.append(probe);
      const value = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return value;
    });

    // Identity first, and it has to be an attribute rather than an index. Hovering MOUNTS and
    // UNMOUNTS controls — a message reveals its action row — so `locator(CONTROL).nth(i)` after
    // the move can resolve to a different element than the one measured at rest, and the pair
    // being compared is then two different controls. Measured while auditing this: for most of
    // a silent list, `elementFromPoint` at the sampled centre returned an unrelated node.
    const total = await page.locator(CONTROL).evaluateAll((nodes) => {
      nodes.forEach((node, index) => node.setAttribute("data-hover-probe", String(index)));
      return nodes.length;
    });

    for (let index = 0; index < total; index += 1) {
      const control = page.locator(`[data-hover-probe="${index}"]`);
      // Park before every reading: sampling a resting state with the pointer still on the
      // previous control reads that one's hover as this one's rest.
      await page.mouse.move(1119, 719);
      await page.waitForTimeout(50);
      const before = await control.evaluate((node) => {
        const box = node.getBoundingClientRect();
        return {
          rest: getComputedStyle(node).backgroundColor,
          // Measured with the pointer parked, so this is the RESTING box — the one the centre
          // has to be taken from. Sampling every box up front and then moving many times reads
          // coordinates from a layout that earlier hovers have already changed.
          x: box.x + box.width / 2,
          y: box.y + box.height / 2,
          width: box.width,
          height: box.height,
          // A disabled control is meant not to answer, and says so with the cursor.
          disabled: node.matches(':disabled, [aria-disabled="true"]'),
          label: (node.getAttribute("aria-label") ?? node.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: node.tagName.toLowerCase(),
        };
      });
      if (before.width < 2 || before.height < 2 || before.disabled) continue;
      if (before.x < 0 || before.y < 0 || before.x > 1120 || before.y > 720) continue;

      // Not `locator.hover()`: it waits for the element to be able to receive pointer events,
      // and a control something else covers never becomes actionable, so the walk hangs on it
      // rather than reporting it.
      await page.mouse.move(before.x, before.y);
      await page.waitForTimeout(70);
      const after = await control.evaluate((node) => ({
        fill: getComputedStyle(node).backgroundColor,
        // Proof the pointer arrived. Without it a control the layout moved out from under the
        // cursor reports "no change" and reads as a control that ignores the pointer.
        reached: node.matches(":hover"),
      }));
      if (!after.reached) {
        unreachable.push(`${route} <${before.tag}> "${before.label}"`);
        continue;
      }
      hovered += 1;
      if (after.fill === before.rest) continue;

      answers.set(`${before.rest} -> ${after.fill}`, `${route} <${before.tag}> "${before.label}"`);
      // `rgba(0, 0, 0, 0)` is the computed spelling of no fill at all: ink over nothing is the
      // wash doing exactly its job. Anything else had a fill for the ink to sit on.
      if (before.rest !== "rgba(0, 0, 0, 0)" && after.fill === neutralWash) {
        replaced.push(
          `${route} <${before.tag}> "${before.label}"  ${before.rest} -> ${after.fill}`,
        );
      }
    }

    await page.mouse.move(1119, 719);
  }

  // A sweep that hovered nothing agrees with everything. Counted on ARRIVAL — `:hover` on the
  // element itself — so a walk that aimed at stale coordinates cannot clear this floor.
  console.log(`hovered ${hovered}, pointer never arrived on ${unreachable.length}`);
  expect(hovered, "the sweep has to reach real controls").toBeGreaterThan(100);
  expect(
    [...new Set(replaced)],
    "hover replaced a resting fill with the neutral wash — lay the ink over that fill instead",
  ).toEqual([]);
  const distinct = [...answers].map(([answer, where]) => `${answer}  first at ${where}`).sort();
  expect(
    distinct.length,
    `one gesture, a closed set of answers:\n${distinct.join("\n")}`,
  ).toBeLessThanOrEqual(MAX_DISTINCT_ANSWERS);
});
