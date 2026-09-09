import { expect, test } from "./test";
import { FOCUSABLE } from "./controls";

// One rule draws every focus ring: 1.5px at `outline-offset: 1px`, so it reaches 2.5px past the
// border box. `[data-focus-inset]` is the compensation for a control flush against something
// that clips — it draws the ring inward instead. The compensation existed, was commented, and
// had **no users**, while the tool-summary disclosure that fills a rounded `overflow-clip`
// container showed a keyboard user no ring at all.
//
// Geometry, not painting: the ring is suppressed unless the last input device was a key
// (`html:not([data-pointer])`), so what THIS test checks is whether it would have anywhere to
// go. The test below it walks the same tree and asks whether it goes there — the question this
// file did not ask, and the answer was no for 109 of 121 controls (see that test's note).
//
// Two refinements this needed before it said anything true, both about scrolling. An element
// scrolled out of its own container reports a 1000px "cut" and is not a defect, so only a ring
// poking out of a box the ELEMENT fits inside counts. And a scrollable ancestor cannot pin
// anything at all: focus scrolls the control clear of the edge, which is why the walk stops at
// the first scroller instead of blaming the clip beyond it.

const ROUTES = [
  "fixture=agent&state=waiting",
  "fixture=agent&state=running",
  "fixture=agent&state=narrative",
  "fixture=agent&state=tool-shells",
  "fixture=agent&state=long-content",
  "fixture=shell&state=populated",
  "fixture=workspace&state=dock-light",
  "fixture=workspace&state=settings",
  "fixture=shell&state=populated&overlay=finder",
  "fixture=shell&state=populated&overlay=commands",
];

test("no focus ring is cut off by something that clips", async ({ page }) => {
  const cut: string[] = [];
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);
    reached += await page.locator(FOCUSABLE).count();

    const found = await page.evaluate((FOCUSABLE) => {
      const REACH = 2.5;
      const out: string[] = [];
      for (const element of document.querySelectorAll(FOCUSABLE)) {
        if (element.hasAttribute("data-chrome-focus")) continue;
        if (element.hasAttribute("data-focus-inset")) continue;
        const tag = element.tagName.toLowerCase();
        if (tag === "input" || tag === "textarea" || (element as HTMLElement).isContentEditable) {
          continue;
        }
        const style = getComputedStyle(element);
        if (style.visibility === "hidden" || style.display === "none") continue;
        const box = element.getBoundingClientRect();
        if (box.width === 0 || box.height === 0) continue;

        for (let parent = element.parentElement; parent; parent = parent.parentElement) {
          const parentStyle = getComputedStyle(parent);
          // A scrollable ancestor cannot pin anything: focusing scrolls the control clear of
          // the edge, honouring `scroll-padding`. Only a box that CANNOT scroll traps a ring,
          // so the walk stops at the first scroller rather than blaming the clip beyond it.
          const scrolls =
            parentStyle.overflowY === "auto" ||
            parentStyle.overflowY === "scroll" ||
            parentStyle.overflowX === "auto" ||
            parentStyle.overflowX === "scroll";
          if (scrolls) break;
          const clipsX = parentStyle.overflowX !== "visible";
          const clipsY = parentStyle.overflowY !== "visible";
          if (!clipsX && !clipsY) continue;
          const clip = parent.getBoundingClientRect();
          const inside =
            box.top >= clip.top - 0.5 &&
            box.bottom <= clip.bottom + 0.5 &&
            box.left >= clip.left - 0.5 &&
            box.right <= clip.right + 0.5;
          if (!inside) continue;
          const bleed = Math.max(
            clipsY ? clip.top - (box.top - REACH) : 0,
            clipsY ? box.bottom + REACH - clip.bottom : 0,
            clipsX ? clip.left - (box.left - REACH) : 0,
            clipsX ? box.right + REACH - clip.right : 0,
          );
          if (bleed <= 0.25) continue;
          out.push(
            `${Math.round(bleed * 10) / 10}px cut from <${tag} class="${(element.getAttribute("class") ?? "").slice(0, 48)}"> ` +
              `by ${parent.tagName.toLowerCase()}.${(parent.getAttribute("class") ?? "").slice(0, 36)}`,
          );
          break;
        }
      }
      return out;
    }, FOCUSABLE);
    cut.push(...found);
  }

  expect(reached, "the sweep has to be looking at real controls").toBeGreaterThan(20);
  expect(
    [...new Set(cut)],
    "focus rings with nowhere to draw — mark the control `data-focus-inset`",
  ).toEqual([]);
});

// Whether a control shows a ring is decided in globals.css, by two attributes it reads. The
// question this file never asked is whether the decision reaches the screen — and it did not.
// A StyleX declaration carries three `:not(#\#)`, so `outline: "none"` in a style object beats
// the global rule at (3,n,0) against (0,4,3). Twenty call sites had written it, most by way of
// `Button`, and 109 of 121 keyboard-reachable controls that had NOT opted out showed nothing at
// all. Under Tailwind the same line was a `@layer utilities` rule the global one beat, so it
// was genuinely harmless there and the chrome guard blessed it in writing; the migration
// inverted it silently, because a focus ring is invisible until someone reaches for the keyboard.
//
// Two assertions, because "a ring appeared" is the weaker claim. Every ring in the product also
// has to be the SAME ring: one fingerprint of style, width and colour across every control, or
// the one-rule-draws-it-all design is already gone whatever the pixels say.
//
// Real Tab, not `element.focus()` — programmatic focus does not run the roving-tabindex
// activation a dock tab uses, for the reasons `chromeFocus.visual.spec.ts` sets out.
const TAB_STEPS = 45;
const STEP_BUDGET_MS = 220;

test("the ring the design promises is the ring that paints", async ({ page }) => {
  test.setTimeout(ROUTES.length * TAB_STEPS * STEP_BUDGET_MS + 20_000);
  const silent: string[] = [];
  const fingerprints = new Map<string, string>();
  let reached = 0;

  for (const route of ROUTES) {
    await page.goto(`/visual/?${route}&theme=light`);
    await page.waitForSelector("html[data-visual-ready]");
    await page.waitForTimeout(200);
    const seen = new Set<string>();

    for (let step = 0; step < TAB_STEPS; step += 1) {
      await page.keyboard.press("Tab");
      await page.waitForTimeout(40);
      const meta = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement | null;
        if (!active || active === document.body) return null;
        const tag = active.tagName.toLowerCase();
        // The three exclusions the global rule itself carries: an opt-out promising a row
        // state instead (`chromeFocus.visual.spec.ts` holds that promise), and text inputs,
        // which say where the keyboard is with a caret.
        if (active.hasAttribute("data-chrome-focus")) return null;
        if (tag === "input" || tag === "textarea" || active.isContentEditable) return null;
        if (!active.matches(":focus-visible")) return null;
        const style = getComputedStyle(active);
        return {
          key: `${active.getAttribute("class") ?? ""}|${(active.textContent ?? "").trim().slice(0, 24)}`,
          tag,
          ring: `${style.outlineStyle} ${style.outlineWidth} ${style.outlineColor}`,
          drawn: style.outlineStyle !== "none",
          label: (active.getAttribute("aria-label") ?? active.textContent ?? "")
            .trim()
            .replace(/\s+/g, " ")
            .slice(0, 34),
        };
      });
      if (!meta || seen.has(meta.key)) continue;
      seen.add(meta.key);
      reached += 1;
      if (meta.drawn) fingerprints.set(meta.ring, `${route} <${meta.tag}> "${meta.label}"`);
      else silent.push(`${route}  <${meta.tag}> "${meta.label}"`);
    }
  }

  // A walk that reached nothing keeps no promise and reports no failure.
  expect(reached, "the walk has to arrive at real controls").toBeGreaterThan(60);
  expect(
    [...new Set(silent)],
    "controls the design promises a ring and that show none — a call site is out-specifying globals.css",
  ).toEqual([]);
  expect(
    [...fingerprints].map(([ring, where]) => `${ring}  first at ${where}`),
    "one rule draws every focus ring, so there is one ring",
  ).toHaveLength(1);
});
