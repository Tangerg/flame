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
// The focus ring went the same way twice, in opposite directions. First sixteen
// callsites drew a second, louder ring over the global one, in seven spellings and
// ungated by pointer modality. Then, once those were gone, twenty callsites turned
// the remaining one OFF — see the focus rule below for why that inverted when
// Tailwind left, and for what it cost.
//
// They live in one place each now: `--color-hover`, `--color-selected`,
// `--press-scale`, the `--dur-*` ladder and the global focus rule in globals.css.
// A hand-picked state colour, a surface swap over a transparent rest state, a
// literal press amount or duration, an all-property transition, a hover answered
// with opacity, or a focus ring drawn or suppressed at a callsite is that decision
// leaking back out.
//
// Every pattern here reads StyleX and CSS. They were Tailwind class regexes, and
// when Tailwind was removed eight of the nine went blind at once while the guard
// went on reading 1258 files and printing the same confident line.
//
// Escape hatch: none by design. A state fill the two tokens cannot express is a
// signal that the interaction model needs a third state, not that this callsite
// needs its own alpha — add it in globals.css so everything else can reach it.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

const RULES = [
  {
    // A state fill with a hand-picked colour. Under Tailwind this read `hover:bg-fg/[0.04]`
    // and the alpha was the tell; in StyleX the value sits under a state KEY, so the tell is
    // a colour literal where a token belongs.
    pattern: /: *"(?:rgba?\(|hsla?\(|oklch\(|oklab\(|color-mix\(|#[0-9a-fA-F]{3})/g,
    message: "hand-picked state colour — use the `surface.hover` / `surface.selected` tokens",
    appliesTo: (line, rel) =>
      rel !== "styles/globals.css" &&
      /":(?:hover|active|focus|focus-visible|focus-within)"|":is\(\[(?:data|aria)-[a-z-]+/.test(
        line,
      ),
  },
  {
    // A surface step as a hover over something with no resting fill: it paints a
    // slab where there was none, and reads heavier in one theme than the other.
    // (A control that already HAS a surface fill may step up — that is `soft`.)
    pattern: /":hover": surface\.surface\d/g,
    message:
      "surface swap as a hover — use `surface.hover` (an ink wash) on a transparent rest state",
    appliesTo: (line) => line.includes('default: "transparent"') || line.includes("default: null"),
  },
  {
    // A swatch and a splitter are not Buttons and should not have to become one
    // to press — but there is only one press amount in the app. `1` is the identity,
    // used to cancel the press on a disabled control, not a value.
    pattern: /:active(?:\)[^"]*)?": *"?0*\.\d+/g,
    message: 'literal press amount — use `":active": "var(--press-scale)"`',
    appliesTo: () => true,
  },
  {
    // The ring is one rule in globals.css; whether a control gets one is decided THERE, by two
    // attributes it reads — `data-focus-inset` when the ring would land outside a box that
    // clips, `data-chrome-focus` when a row state stands in for it.
    //
    // A call site cannot participate in that decision, only overrule it. Under Tailwind
    // `focus-visible:outline-none` was a `@layer utilities` rule that the unlayered global one
    // beat, so suppressing the browser default at a call site was harmless and this guard said
    // so in as many words. StyleX inverted it: every declaration carries three `:not(#\#)`, so
    // `outline: "none"` in a style object outranks the global rule at (3,n,0) against (0,4,3)
    // and takes the design's ring down with the browser's. Twenty call sites said it, most of
    // them on `Button` — 109 of 121 keyboard-reachable controls that had NOT opted out showed
    // nothing at all, and the two audits either side of this both looked past it: one checks
    // the ring has room to draw, the other checks the opt-outs keep their promise.
    //
    // A real outline VALUE is a different statement and stays legal — a bare field borrows one
    // to mark itself invalid, and `input` is excluded from the ring rule by selector.
    pattern: /\boutline(?:Style|Width)?: *(?:"none"|0\b|none;)|\boutline[A-Za-z]*: *\{[^}]*"none"/g,
    message:
      "the ring's suppression belongs in globals.css — a StyleX `outline` outranks it; use `data-chrome-focus` if a row state stands in",
    // globals.css is exempt for the rules that ARE the focus model, not for the whole file.
    // Exempting the file let two per-component suppressions sit beside them: both pane
    // resizers carried `outline: none` at (0,1,0) against the global rule's (0,4,3), so
    // neither ever suppressed anything, on elements whose own comment says a keyboard user
    // needs to see their ring. One was found by a runtime walk that happened to reach it; its
    // twin was three tab stops past where that walk stopped.
    appliesTo: (_line, rel, selector) =>
      rel !== "styles/globals.css" || !/:focus-visible|\[data-pointer\]/.test(selector),
  },
  {
    // Opacity as a hover answer, in either of its two forms. A REVEAL — transparent at rest,
    // shown on hover — leaves a tab stop that paints nothing, because transparency stops the
    // pointer and not the keyboard; the composer's remove-attachment button was one. Reveals
    // now belong to `reveal.ts`, whose `--reveal` variables answer hover and focus together,
    // so a hand-rolled one at a call site is also answering only half the question.
    //
    // A DIM — full strength at rest, faded on hover — is not a reveal and still wrong: it is
    // a fourth spelling of "the pointer is over this" beside the ink wash, the surface step
    // and the colour step. `TextButton`'s negative tone carried one at 0.8, three variants
    // away from two siblings that say the same thing as a colour.
    pattern: /opacity: \{[^}]*":hover"/g,
    message:
      "hover as an opacity — a reveal belongs to `reveal.ts`'s `--reveal` variables, which answer the keyboard too; a hover STATE is `surface.hover`",
    appliesTo: () => true,
  },
  {
    // `:has(:focus-visible)` says the right thing and does not do it. Chromium matches the
    // selector — `element.matches()` agrees, the rule is in the sheet, it outranks the
    // `opacity-0` beside it — but it never invalidates the subtree when focus-visible
    // changes inside `:has()`, so the reveal only lands when some unrelated recalculation
    // happens to follow. Measured: reattaching the node flipped it from 0 to 1 with nothing
    // else changed. An affordance that appears at random is worse than one that lingers.
    pattern: /:has\(:focus-visible\)/g,
    message:
      "`:has(:focus-visible)` matches but never invalidates in Chromium — key the reveal on the focusable element's own `focus-visible:`, or keep `focus-within:`",
    appliesTo: () => true,
  },
  {
    // `Icon` renders `aria-hidden` and destructures four props; a name handed to it compiles and
    // is thrown away, because TypeScript does not check hyphenated JSX attributes against a
    // component's props. Types pass, tests pass, and only a screen reader notices — so the
    // callsite claims the role on a wrapper instead.
    pattern: /<Icon\b[^>]*\saria-label=/gs,
    message: '`Icon` drops a name it accepts — wrap it in `<span role="img" aria-label=…>`',
    appliesTo: () => true,
  },
  {
    // "This action is in flight" is not "this action is unavailable", and only the tab order
    // shows the difference. `disabled` is enforced by the platform making the element
    // unfocusable, so a control that disables itself while its own work runs blurs whoever was
    // standing on it: measured on Settings → Connection, pressing Enter on Refresh put focus on
    // `<body>` 120ms later and left it there. Forty-four call sites said it this way under eight
    // flag names, which is every async action in the product dropping the keyboard user's place.
    //
    // `pending` on the button primitive says it with `aria-disabled` instead — same words to a
    // screen reader, same appearance, still a tab stop, and the click refused in the component.
    //
    // The flag names are a list rather than a shape, because "in flight" has no syntax. It is
    // the eight this codebase actually used, so a ninth spelling passes; a name is cheaper to
    // add here than the defect is to find. A genuinely unavailable action — an invalid form, a
    // row with nothing selected — is still `disabled`, and combining the two in one expression
    // is what hid this: `!valid || saving` reads as one fact and is two.
    pattern:
      /disabled=\{[^}]*\b(?:busy|saving|signingIn|reconnecting|importing|submitting|testing|inFlight)\b/g,
    message:
      "in-flight spelled as `disabled` — it un-focuses the control the user just activated; use `pending`",
    // The list is what has a `pending` to move to. Buttons START the work; fields and choice
    // lists are what it is started FROM, and they lost focus the same way — measured on the
    // relocate banner, where typing a path and pressing Enter put focus on `<body>`. A field's
    // `pending` is `readOnly` as well as `aria-disabled`, because `aria-disabled` alone leaves
    // it typable.
    //
    // `Switch` is deliberately absent: measured on Settings → Schedules, toggling it never
    // moved focus, so it has no defect to fix and `aria-disabled` would leave it togglable.
    appliesTo: (_line, _rel, _selector, jsxTag) =>
      [
        "Button",
        "PillButton",
        "TextButton",
        "IconButton",
        "BannerAction",
        "TextField",
        "TextArea",
        "ChoiceList",
        "ChoiceOption",
      ].includes(jsxTag),
  },
  {
    pattern: /transition(?:Property|-property)?: *"?[^";]*\ball\b/g,
    message:
      "`transition-all` couples unrelated properties — enumerate only the properties that move",
    appliesTo: () => true,
  },
  {
    // Transitions only. An `animation` is a cycle, not interaction feedback — the caret's blink
    // and the scrim's fade are longer than every rung on the ladder and mean something else;
    // both already wrap their literal in `calc(… * var(--motion-scale))`, which is the part
    // that has to be true.
    pattern: /transition(?:Duration|Delay|-duration|-delay)?: *"?[^";]*\b\d+m?s\b/g,
    message: "literal transition duration — use the semantic `--dur-*` ladder",
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
  // Which rule a CSS declaration belongs to, so an exemption can name a rule instead of a
  // file. Only the line carrying the brace is needed: a selector prettier has wrapped keeps
  // its most specific part there.
  let selector = "";
  // Which ELEMENT a JSX prop belongs to. Same reason the CSS selector is tracked above: a prop
  // on a multi-line element sits on a line with no tag on it, so a rule that reads only its own
  // line cannot tell a `<Switch>` from a `<Button>` — and the in-flight rule below has to,
  // because a switch keeps `disabled` and a button does not. Reading the line alone flagged all
  // three form controls in the tree.
  let jsxTag = "";
  let inBlockComment = false;
  lines.forEach((line, index) => {
    // A `/* */` block's CONTINUATION lines open with prose, not with a comment marker, so the
    // single-line skip below does not cover them — and two of these rules document themselves
    // by spelling out the pattern they forbid. The rule that reads globals.css was reporting
    // the paragraph explaining why it exists.
    const wasInComment = inBlockComment;
    for (const marker of line.match(/\/\*|\*\//g) ?? []) {
      inBlockComment = marker === "/*";
    }
    if (wasInComment || /\/\*/.test(line)) return;
    const opensTag = [...line.matchAll(/<([A-Z][A-Za-z0-9.]*)/g)].pop();
    if (opensTag) jsxTag = opensTag[1];
    if (extname(path) === ".css") {
      const opens = /^([^{}]*)\{\s*$/.exec(line);
      if (opens) selector = opens[1].trim();
      else if (line.trim() === "}") selector = "";
    }
    // A line that opens as a comment styles nothing, and two of these rules are documented by
    // spelling out the pattern they forbid — so a guard reading prose flags its own warning.
    if (/^\s*(?:\/\/|\*|\/\*)/.test(line)) return;
    for (const { pattern, message, appliesTo } of RULES) {
      if (!appliesTo(line, rel, selector, jsxTag)) continue;
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
