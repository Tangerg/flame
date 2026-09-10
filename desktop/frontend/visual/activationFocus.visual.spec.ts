import { test as base, expect } from "@playwright/test";

// Activating a control must never leave focus on nothing.
//
// `<body>` is not a neutral resting place: the next Tab restarts at the top of the document, so
// a keyboard user who presses a button loses their place in it entirely. Three separate causes
// of it have been found in this product — a control that disabled itself, a field that disabled
// itself, and a message action whose whole bar was removed by the state its own click produced.
// The first two have their own audits; this one asks the question at the end, about every
// control, without caring which cause it is.
//
// THE PRECONDITION IS THE WHOLE AUDIT. The first version of this swept the dock and reported
// thirteen controls orphaning focus. Twelve were its own fault: a dock tab's × is
// `visibility: hidden` until hover, `focus()` on it is a silent no-op, so focus stayed on
// `<body>` where the page load left it and pressing Enter proved nothing. The reading was
// "wherever focus already was" dressed up as a finding. So every iteration now asserts that
// focus actually LANDED before activating, and a control that cannot take focus is reported
// separately rather than counted as a defect.
//
// (The × being unreachable is not a defect either: a focusable sibling inside a `tablist` is an
// axe-critical `aria-required-children` violation, so Delete/Backspace on the focused tab is the
// ARIA practice instead. Verified working — it closes the tab and moves focus to the next one.)
//
// No `quietConsole`: this activates every control it can reach, and in a fixture with no runtime
// behind it the ones that call out complain. That noise is expected and is not what this
// measures, so the console check here allows exactly it and nothing else.
const EXPECTED_NOISE = /RpcConnectionError|Failed to fetch|net::ERR_CONNECTION_REFUSED/;

const ROUTES = [
  "fixture=agent&state=narrative",
  "fixture=workspace&state=dock-agent-memory",
  "fixture=shell&state=populated",
] as const;

const PER_ROUTE = 45;

base("activating a control never leaves focus on nothing", async ({ page }) => {
  base.setTimeout(ROUTES.length * 90_000 + 30_000);
  const unexpected: string[] = [];
  page.on("console", (message) => {
    if (message.type() !== "error" && message.type() !== "warning") return;
    if (EXPECTED_NOISE.test(message.text())) return;
    unexpected.push(`${message.type()}: ${message.text()}`);
  });

  await page.setViewportSize({ width: 1472, height: 900 });
  const orphaned: string[] = [];
  const unfocusable: string[] = [];
  let activated = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.locator("html[data-visual-ready]").waitFor();
    await page.waitForTimeout(300);

    const total = await page.locator('[data-slot="button"]:not([data-fixture-chrome] *)').count();
    for (let index = 0; index < Math.min(total, PER_ROUTE); index += 1) {
      const control = page.locator('[data-slot="button"]:not([data-fixture-chrome] *)').nth(index);
      if ((await control.count()) === 0) continue;
      const name = await control.evaluate(
        (node) =>
          `${(node.textContent ?? "").trim().slice(0, 18)}|${node.getAttribute("aria-label") ?? ""}`,
      );

      // The precondition. A `disabled` control and one hidden behind `visibility: hidden` both
      // refuse focus silently, and neither can orphan anything it never held.
      const landed = await control.evaluate((node) => {
        (node as HTMLElement).focus();
        return document.activeElement === node;
      });
      if (!landed) {
        unfocusable.push(`${route} #${index} "${name}"`);
        continue;
      }

      await page.keyboard.press("Enter");
      await page.waitForTimeout(200);
      activated += 1;
      const landedOn = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement | null;
        return !active || active === document.body ? "BODY" : "somewhere";
      });
      if (landedOn === "BODY") orphaned.push(`${route} #${index} "${name}"`);

      // Whatever the last activation opened, close it, so the next iteration starts from a
      // route that still resembles itself. A stuck overlay would show up as a run of controls
      // that cannot take focus rather than as a wrong answer.
      await page.keyboard.press("Escape");
      await page.waitForTimeout(60);
    }
  }

  // Floor, not a target: an audit that activated nothing agrees with every product.
  //
  // It was 60 when this swept the page. Excluding `[data-fixture-chrome]` — the harness's own
  // state switcher, which is 23 of 51 buttons on the agent route and 28 of 52 on the workspace
  // one — took the real count to 41. The floor is now under the product's own number rather
  // than under a number the test scaffold was helping to reach.
  expect(activated, "the sweep has to actually activate controls").toBeGreaterThan(30);
  expect(orphaned, "controls that left focus on `<body>`").toEqual([]);
  expect(unexpected, "console complaints other than the fixture's missing runtime").toEqual([]);
  // Not asserted, only surfaced: a control that cannot take focus is a different question, and
  // the ones here are answered above.
  if (unfocusable.length > 0) console.log(`not focusable (by design): ${unfocusable.length}`);
});
