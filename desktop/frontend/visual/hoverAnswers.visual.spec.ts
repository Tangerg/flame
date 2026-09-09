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
        const style = getComputedStyle(node);
        // The centre of a control's own rect is not where it is painted. Inside a scroller a
        // control can be scrolled out of view and still report a rect that lands inside the
        // window, so the pointer goes to a point showing something else entirely — thirteen
        // controls read as ignoring the pointer for exactly this reason. Aim at the point that
        // is visible: the control's rect intersected with every ancestor that clips.
        let box = node.getBoundingClientRect();
        let visible = { top: box.top, bottom: box.bottom, left: box.left, right: box.right };
        for (let parent = node.parentElement; parent; parent = parent.parentElement) {
          const parentStyle = getComputedStyle(parent);
          if (parentStyle.overflowX === "visible" && parentStyle.overflowY === "visible") continue;
          const clip = parent.getBoundingClientRect();
          visible = {
            top: Math.max(visible.top, clip.top),
            bottom: Math.min(visible.bottom, clip.bottom),
            left: Math.max(visible.left, clip.left),
            right: Math.min(visible.right, clip.right),
          };
        }
        return {
          rest: style.backgroundColor,
          x: (visible.left + visible.right) / 2,
          y: (visible.top + visible.bottom) / 2,
          width: visible.right - visible.left,
          height: visible.bottom - visible.top,
          // A disabled control is meant not to answer, and says so with the cursor. A control
          // that is not SHOWN is a third thing again: `Jump to bottom` sits at `opacity: 0`
          // with `pointer-events: none` until the transcript is scrolled, and a walk that
          // counts it as ignoring the pointer is counting a control that is not there.
          skip:
            node.matches(':disabled, [aria-disabled="true"]') ||
            // A range input is the accessibility surface of a slider, not its pointer target
            // — Base UI paints `Track` and `Thumb` for that and leaves the input covered. Held
            // here rather than in the shared `CONTROL` list, which two other audits read and
            // for which a slider's input is a real control.
            node.matches('input[type="range"]') ||
            style.pointerEvents === "none" ||
            style.opacity === "0",
          label: (node.getAttribute("aria-label") ?? node.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: node.tagName.toLowerCase(),
        };
      });
      if (before.width < 2 || before.height < 2 || before.skip) continue;
      if (before.x < 0 || before.y < 0 || before.x > 1120 || before.y > 720) continue;

      // Not `locator.hover()`: it waits for the element to be able to receive pointer events,
      // and a control something else covers never becomes actionable, so the walk hangs on it
      // rather than reporting it.
      await page.mouse.move(before.x, before.y);
      // Then a nudge, because arriving is not the same as being hovered. A control revealed by
      // an ancestor's hover is `visibility: hidden` at rest and so not hit-testable; the move
      // reveals it, but `:hover` on it is only recomputed on the next pointer event. Without
      // this the message actions read as ignoring the pointer that had just revealed them.
      await page.mouse.move(before.x + 1, before.y);
      await page.waitForTimeout(70);
      const after = await control.evaluate((node) => {
        const style = getComputedStyle(node);
        return {
          fill: style.backgroundColor,
          // Proof the pointer arrived. Without it a control the layout moved out from under
          // the cursor reports "no change" and reads as a control that ignores the pointer.
          reached: node.matches(":hover"),
          // Read AFTER the move, because being shown is what the move decides for a revealed
          // control. `visibility` inherits, so a control inside a hidden subtree computes
          // `hidden` however its own styles read — which is what the message actions in the
          // dock fixture turned out to be, while a dock tab's × is hidden only until its tab
          // is hovered. Asking before the move cannot tell those two apart.
          shown: style.visibility !== "hidden" && style.opacity !== "0",
        };
      });
      if (!after.reached) {
        if (after.shown) unreachable.push(`${route} <${before.tag}> "${before.label}"`);
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
  console.log(
    `hovered ${hovered}, pointer never arrived on ${unreachable.length}` +
      (unreachable.length > 0 ? `\n  ${unreachable.join("\n  ")}` : ""),
  );
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
