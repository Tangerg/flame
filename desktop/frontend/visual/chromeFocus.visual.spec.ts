import { expect, test } from "./test";

// `data-chrome-focus` turns the focus ring off on a promise: that a row state shows focus
// instead. `menu.tsx` states it — a popup takes focus so the keyboard can drive it, and the
// highlighted ITEM is the indicator. Nothing checked the promise was kept, and three controls
// were not keeping it: the question card was a tab stop that showed nothing, the header diff
// stat had no row around it at all, and the ACTIVE dock tab's stand-in was
// `focus-within:text-fg`, which it already had.
//
// Measured the way a person meets it: press Tab until a control that opted out has focus,
// photograph it, blur, photograph again. Identical bytes mean a keyboard user sees nothing.
//
// Real Tab, not `element.focus()`. Programmatic focus does not run the roving-tabindex
// activation a dock tab uses, so it reported six controls as silent that a keyboard shows
// plainly — and it lands on the first tabbable before the walk starts, which reads as "no
// change" for whatever that happens to be.

const ROUTES = [
  "fixture=agent&state=question",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
];

const SETTLE_MS = 500;

test("a control that turns off the ring shows focus some other way", async ({ page }) => {
  const silent: string[] = [];

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    const seen = new Set<string>();

    for (let step = 0; step < 40; step += 1) {
      await page.keyboard.press("Tab");
      await page.waitForTimeout(110);
      const meta = await page.evaluate(() => {
        const active = document.activeElement;
        if (!active || active === document.body) return null;
        const box = active.getBoundingClientRect();
        return {
          optOut: active.hasAttribute("data-chrome-focus"),
          focusVisible: active.matches(":focus-visible"),
          key: `${active.getAttribute("class") ?? ""}|${(active.textContent ?? "").trim().slice(0, 24)}`,
          label: (
            active.getAttribute("aria-label") ??
            active.getAttribute("title") ??
            active.textContent ??
            ""
          )
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 30),
          tag: active.tagName.toLowerCase(),
          x: box.x,
          y: box.y,
          width: box.width,
          height: box.height,
        };
      });
      if (!meta || !meta.optOut || !meta.focusVisible || seen.has(meta.key)) continue;
      seen.add(meta.key);
      if (meta.width === 0 || meta.y < 0 || meta.y + meta.height > 720) continue;
      if (meta.x + meta.width > 1120) continue;

      const clip = {
        x: Math.max(0, meta.x - 3),
        y: Math.max(0, meta.y - 3),
        width: meta.width + 6,
        height: meta.height + 6,
      };
      const focused = await page.screenshot({ clip });
      await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
      await page.waitForTimeout(SETTLE_MS);
      const blurred = await page.screenshot({ clip });
      if (Buffer.compare(focused, blurred) === 0) {
        silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
      }

      // The walk restarts from the top, because blurring dropped the position in the order.
      await page.evaluate(() => document.body.focus());
      for (let back = 0; back <= step; back += 1) await page.keyboard.press("Tab");
    }
  }

  expect(
    [...new Set(silent)],
    "controls with `data-chrome-focus` that show nothing when a keyboard reaches them",
  ).toEqual([]);
});
