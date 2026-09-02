#!/usr/bin/env node
// Interaction-chrome guard — keeps "the pointer is over this", "this is the
// chosen one", "this is being pressed" and "the keyboard is here" as one value
// each.
//
// Why this exists: these three facts were being decided at the callsite. The
// tree carried eleven distinct spellings of a neutral hover (`fg/2%` through
// `fg/8%`, plus three surface swaps) across 67 places, and four press amounts
// (0.90 / 0.95 / 0.96 / 0.98). The same gesture answered at four times the
// strength depending on which panel you were in — the kind of inconsistency you
// feel before you can name it, and one that no amount of care at the callsite
// can fix, because the callsite is the wrong place to hold the value.
//
// The focus ring had gone the same way, and worse: globals.css has drawn one
// global keyboard ring all along (modality-gated, quiet, `[data-chrome-focus]` to
// opt a row out), and sixteen callsites had drawn a second, louder one over it in
// seven spellings — an outline at three offsets, a two-layer shadow halo, and
// three hand-rolled box-shadow rings at 1.5px and 2px, inset and outset. Those
// fired on mouse clicks too, which the global rule deliberately avoids.
//
// They now live in one place each: `--color-hover`, `--color-selected`,
// `--press-scale`, the `--dur-*` ladder and the global focus rule in globals.css.
// A state-prefixed ink wash, a surface swap over a transparent rest state, a
// literal press amount or duration, `transition-all`, or a hand-drawn focus ring
// is that decision leaking back out to the callsite.
//
// Escape hatch: none by design. A state fill the two tokens cannot express is a
// signal that the interaction model needs a third state, not that this callsite
// needs its own alpha — add it in globals.css so everything else can reach it.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

const RULES = [
  {
    // `hover:bg-fg/[0.04]`, `data-[active]:bg-fg/[0.06]`, `aria-selected:…` —
    // a state fill with a hand-picked alpha.
    pattern:
      /\b(?:hover|focus|focus-visible|focus-within|active|group-hover|group-focus-within|aria-selected|data-\[[a-z-]+\]):bg-fg\/\[[\d.]+\]/g,
    message: "hand-picked state fill — use `bg-hover` or `bg-selected`",
    appliesTo: () => true,
  },
  {
    // A surface step as a hover over something with no resting fill: it paints a
    // slab where there was none, and reads heavier in one theme than the other.
    // (A control that already HAS a surface fill may step up — that is `soft`.)
    pattern: /\bhover:bg-surface(?:-\d)?\b/g,
    message: "surface swap as a hover — use `bg-hover` (an ink wash) on a transparent rest state",
    appliesTo: (line) => line.includes("bg-transparent"),
  },
  {
    // A swatch and a splitter are not Buttons and should not have to become one
    // to press — but there is only one press amount in the app. `scale-100` is
    // the identity, used to cancel the press on a disabled control, not a value.
    pattern: /\bactive:scale-(?:\[(?!var\(--press-scale\)|1\])[\d.]+\]|(?!100\b)\d+)/g,
    message: "literal press amount — use `active:scale-[var(--press-scale)]`",
    appliesTo: () => true,
  },
  {
    // A focus ring drawn at the callsite: an accent outline, or a box-shadow ring
    // in accent. The global rule already draws one for every focusable element —
    // mark `data-focus-inset` if it would land outside the box, or
    // `data-chrome-focus` if the control is a row that fills instead.
    // `focus-visible:outline-none` stays legal — that suppresses the browser
    // default on a wrapper that isn't the focus target.
    // `ring-*` belongs here too: three call sites drew `focus-visible:ring-2` and it was
    // invisible in review because the accompanying `ring-focus` named a colour the theme
    // never defined, so the second ring came out in currentColor — and, unlike the global
    // rule, ungated by pointer modality, so it also fired on a click.
    pattern:
      /\bfocus(?:-visible|-within)?:(?:outline-(?:accent|offset-[[\]\d.px-]+)|ring(?:-[a-z0-9]+)?\b|shadow-\[[^\]]*--color-accent[^\]]*\])/g,
    message:
      "hand-drawn focus ring — the global rule draws it; mark `data-focus-inset` if it would clip",
    appliesTo: (_line, rel) => rel !== "styles/globals.css",
  },
  {
    // A control revealed by hover and by nothing else. Transparency stops the pointer
    // and not the keyboard, so the control stays a tab stop that paints nothing: the
    // composer's remove-attachment button was one. Four other reveals in the tree pair
    // the hover with a focus variant; this makes the pair the rule, and it has to be on
    // the same line so a reader can see both at once.
    pattern: /\b(?:group-)?hover(?:\/[a-z-]+)?:opacity-100\b/g,
    message: "hover-only reveal — pair it with `focus-within:` / `focus-visible:opacity-100`",
    // …unless the thing is hidden with `visibility`, which the keyboard cannot reach at all:
    // pairing a focus variant onto one of those writes a rule that can never fire, and the
    // dock's close control carried exactly such a line. `visibility: hidden` is the right
    // way to say "pointer-only affordance" when the action has its own key elsewhere — that
    // one closes its tab on Delete, because a focusable sibling inside a `tablist` is an
    // unallowed child. No regex can check that the key exists, so a test holds it instead.
    appliesTo: (line) =>
      !/(?:group-)?focus(?:-within|-visible)?(?:\/[a-z-]+)?:opacity-100/.test(line) &&
      !/\binvisible\b/.test(line),
  },
  {
    // `:has(:focus-visible)` says the right thing and does not do it. Chromium matches the
    // selector — `element.matches()` agrees, the rule is in the sheet, it outranks the
    // `opacity-0` beside it — but it never invalidates the subtree when focus-visible
    // changes inside `:has()`, so the reveal only lands when some unrelated recalculation
    // happens to follow. Measured: reattaching the node flipped it from 0 to 1 with nothing
    // else changed. An affordance that appears at random is worse than one that lingers.
    pattern: /\b(?:group-)?has-\[:focus-visible\]/g,
    message:
      "`:has(:focus-visible)` matches but never invalidates in Chromium — key the reveal on the focusable element's own `focus-visible:`, or keep `focus-within:`",
    appliesTo: () => true,
  },
  {
    pattern: /\btransition-all\b/g,
    message:
      "`transition-all` couples unrelated properties — enumerate only the properties that move",
    appliesTo: () => true,
  },
  {
    pattern: /\bduration-(?:\d+\b|\[[\d.]+ms\])/g,
    message: "literal interaction duration — use the semantic `--dur-*` ladder",
    appliesTo: (_line, rel) => rel !== "styles/globals.css",
  },
];

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

const violations = [];
let examined = 0;
for (const path of walk(SRC)) {
  if (![".ts", ".tsx", ".css"].includes(extname(path))) continue;
  examined += 1;
  const rel = relative(SRC, path);
  const lines = readFileSync(path, "utf8").split("\n");
  lines.forEach((line, index) => {
    for (const { pattern, message, appliesTo } of RULES) {
      if (!appliesTo(line, rel)) continue;
      for (const match of line.matchAll(pattern)) {
        violations.push(`${rel}:${index + 1}  ${match[0]}  — ${message}`);
      }
    }
  });
}

if (violations.length > 0) {
  console.error(`check-interactive-chrome: ${violations.length} callsite-decided state(s)\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}
// Floor, not a target: a guard that read nothing prints the same OK as one that read everything.
const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-interactive-chrome: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}
console.log(
  `check-interactive-chrome: ${examined} files read; hover + selected + press + focus + motion each hold one value`,
);
