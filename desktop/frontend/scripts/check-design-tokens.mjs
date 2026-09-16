#!/usr/bin/env node
// Design-token guard — keeps type size, line height and corner radius on their
// ladders.
//
// Why this exists: before the ladders landed, the tree carried 16 distinct
// hardcoded `text-[Npx]` values across ~390 callsites, 12 distinct
// `rounded-[Npx]` values, and five line heights within 0.2 of each other
// (1.4 / 1.45 / 1.5 / 1.55 / 1.6) across 53 callsites. That is how a UI ends up
// with 11px next to 11.5px. All three are now derived in globals.css (`--fs-*` /
// `--leading-*` / `--shape-*`), so an arbitrary value at a callsite is a
// regression: it silently opts that one element out of the ladder, and out of the
// user's size and shape preferences.
//
// Escape hatch: none by design. A size, radius or tint the ladder cannot express is a
// signal the ladder needs a step, not that this callsite needs an exception —
// add the step in globals.css so every other callsite can reach it too.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

// The widths the product has named. Derived rather than listed: a token that is renamed or
// re-valued must break the floor below instead of quietly leaving the rule matching nothing,
// which is how this file's Tailwind-era rules went silent when the class names left.
const GLOBALS = readFileSync(new URL("../src/styles/globals.css", import.meta.url), "utf8");
const NAMED_EDGE_WIDTHS = [
  ...new Set(
    [...GLOBALS.matchAll(/^\s*--(?:control-edge|hairline)-width:\s*([\d.]+px);/gm)].map(
      (match) => match[1],
    ),
  ),
];

const MARKUP_RULES = [
  {
    // Every rule here reads a STYLE OBJECT, because that is what this codebase writes.
    //
    // Eleven of the rules this file used to carry were regexes over Tailwind class strings, and
    // all eleven had matched nothing since the utilities left: `bg-<tone>/N`, `bg-fg/N`,
    // `text-[Npx]`, `text-sm`, `shadow-[...]`, `border-fg/N`, `rounded-[Npx]`, `z-NN`,
    // `leading-[N]` and the black/white and ambiguous-border-slot pair. A green check on a rule
    // that cannot fire is worse than no rule, so the ones with a StyleX shape are written here
    // and the ones that were Tailwind grammar are gone.
    //
    // `text-sm` and the ambiguous `border-[var(--…)]` slot were the latter: one names Tailwind's
    // own size scale and the other exists because Tailwind cannot tell a length from a colour
    // inside a bare `var()`. Neither means anything in a style object.
    //
    // `bg-<tone>/N`, `bg-fg/N` and `border-fg/N` were three spellings of one defect — a token
    // tinted at a hand-picked alpha — and they are the `color-mix` rule at the end of this list.
    pattern: /fontSize:\s*"[\d.]+(?:px|rem)"/g,
    message: "arbitrary font size — compose a `type.*` step from `tokens.stylex.ts`",
  },
  {
    // `1` is exempt, and was exempt before: it is the glyph box, which a mark or a badge asks
    // for so its own height is the type's. It is not a step on the reading ladder.
    pattern: /lineHeight:\s*(?!1\s*[,}])[\d.]+\s*[,}]/g,
    message: "arbitrary line height — use a `leading.*` step",
  },
  {
    // A cast written out cannot follow the visual style, which is what `--shadow-*` is for.
    pattern: /boxShadow:\s*"(?!none"|var\()/g,
    message: "hardcoded shadow — define a `--shadow-*` token in globals.css and use it",
  },
  {
    pattern: /borderRadius:\s*"[\d.]+(?:px|rem)"/g,
    message: "arbitrary corner radius — use a `radius.*` step",
  },
  {
    // A double-digit layer. Within one stacking context a handful of steps is all there is to
    // order, so anything reaching past single digits is competing with the window — and that
    // competition has exactly two rungs, both named. Seven spellings had accumulated across five
    // files, which is how the session search panel came to sit at the same height as an open menu.
    pattern: /zIndex:\s*(?:\d\d+|"(?!var\(--layer-)[^"]*")/g,
    message: "a layer competing with the window — use `var(--layer-*)`",
  },
  {
    // One pixel is exempt: a hairline gap and a baseline nudge are not ladder steps.
    pattern:
      /\b(?:padding|margin)(?:Block|Inline|Top|Right|Bottom|Left)?(?:Start|End)?:\s*"(?!1px)[\d.]+px"/g,
    message: "a spacing literal — name the `space` step, minus an edge if that is what it is",
    appliesTo: (rel) => rel.startsWith("ui/"),
  },
  {
    // A duration written at a motion call site cannot scale. `lib/motion` publishes every
    // preset with a live getter so the user's motion preference reaches each animation with
    // no hook per call site — and the one call site that spelled its own out went on
    // animating for 25 style frames with that preference at zero, alone in the app.
    // `0.3` was this ladder's `slowMs` all along.
    // Prettier breaks the object as soon as it carries a second key, so the line-by-line pass
    // this file is built on reads `duration:` with no `transition` in sight. The rule is scanned
    // against whole file text instead — the one shape it exists to catch is the one that wraps.
    pattern: /transition\s*[=:]\s*\{\{?[^}]*\bduration:\s*[\d.]+/g,
    spansLines: true,
    message: "literal animation duration — use a preset from `lib/motion`, or add the rung there",
    appliesTo: (rel) => rel !== "lib/motion.ts",
  },
  {
    // An edge spelled out where a token already names that exact width. The product draws two
    // on purpose — a control states its bounds at `--control-edge-width`, something that only
    // wants separating from what it sits on takes `--hairline-width` — and both are visual-style
    // tokens, so a literal silently opts its one element out of the style. Twenty-two call sites
    // had one, in the same two values, which meant a style that moved either width would have
    // moved it for some of the product's edges and not the rest.
    //
    // Only the NAMED values: the composer's 2px, a swatch ring's 2px and a pending mark's 1.5px
    // are one-of-a-kind strokes, and a token per single call site is the drift this file is for.
    pattern: new RegExp(
      String.raw`border(?:Top|Bottom|Left|Right|Block|Inline)?(?:Start|End)?Width:\s*"(?:` +
        NAMED_EDGE_WIDTHS.map((value) => value.replace(".", String.raw`\.`)).join("|") +
        String.raw`)"`,
      "g",
    ),
    message:
      "an edge width a token already names — use `var(--control-edge-width)` or `var(--hairline-width)`",
  },
  {
    // HALF a type step, copied. A step is a bundle — `--text-display-md` carries a size, a
    // tracking AND a leading — and a call site that reaches for the size variable takes one
    // third of it. Six of them did, which at the largest font size left 26px headings sitting
    // on the transcript's own leading. `tokens.stylex.ts` is where the bundle is authored.
    pattern: /fontSize:\s*"var\(--(?:text|fs)-[^)]*\)"/g,
    message:
      "half a type step — compose `type.uiMd` / `type.displayMd` from `tokens.stylex.ts`, which carries the tracking and leading with it",
    appliesTo: (rel) => rel !== "styles/tokens.stylex.ts",
  },
  {
    // An ink or accent wash mixed by hand in an arbitrary value. The mermaid
    // block had built two panels this way, out of four alphas of its own — which
    // also opted them out of the contrast preference, since `--depth-step` is
    // what moves the ladder.
    pattern: /color-mix\([^)]*var\(--(?:color-(?:text|accent)|tone-\w+)\)[^)]*\)/g,
    message:
      "hand-mixed token alpha — use a ladder step (`surface.*` / `surface.field*` / `surface.<tone>Wash`)",
    // The theme kit is where the ladder's values are authored — mixing is its job.
    appliesTo: (rel) => !rel.startsWith("plugins/builtin/theme/"),
  },
];

const STYLESHEET_RULES = [
  {
    pattern: /font-size:\s*[\d.]+(?:px|rem)/g,
    message: "arbitrary font size — use `var(--fs-*)`",
  },
  {
    pattern: /border-radius:\s*[\d.]+(?:px|rem)/g,
    message: "arbitrary corner radius — use `var(--shape-*)`",
  },
  {
    pattern: /line-height:\s*[\d.]+\s*;/g,
    message: "arbitrary line height — use `var(--leading-*)`",
  },
  {
    // A literal colour in a consuming stylesheet answers "which colour" where the
    // theme should: it can't follow a scheme, a contributed theme, or contrast.
    // The active search hit had painted `color: #000` this way.
    pattern: /#(?:[\da-fA-F]{3,4}|[\da-fA-F]{6}|[\da-fA-F]{8})\b/g,
    message: "literal colour — define a token in globals.css and use `var(--color-*)`",
  },
];

// Holds in EVERY stylesheet, globals.css included, because a ladder whose owner is exempt
// from it is not a ladder. The rungs are declared as `--layer-*` custom properties, so a
// literal `z-index` here is always a consumer and never a definition — unlike `--fs-*` or
// `--shape-*`, where globals.css genuinely has to spell the number out.
//
// The hole this closes: the markup rule below refused a double-digit `z-` at every call
// site while the stylesheet carried six of them — 40, 30, 10, 25, 15, 25 — for the whole
// shell order, with 25 landing on two elements by coincidence rather than by declaration.
// Single digits stay legal: `z-index: 1` means "above my own siblings" inside a local
// stacking context and takes part in no order beyond it.
const EVERY_STYLESHEET_RULES = [
  {
    pattern: /z-index:\s*-?\d\d+/g,
    message: "unnamed stacking rung — declare a `--layer-*` step and use `var(--layer-*)`",
  },
];

// globals.css owns the ladders, so it is the one file allowed to spell the
// numbers out.
const STYLESHEET_EXEMPT = new Set(["styles/globals.css"]);

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

function rulesFor(path) {
  const ext = extname(path);
  if (ext === ".tsx" || ext === ".ts") return MARKUP_RULES;
  if (ext === ".css") {
    return STYLESHEET_EXEMPT.has(relative(SRC, path))
      ? EVERY_STYLESHEET_RULES
      : [...EVERY_STYLESHEET_RULES, ...STYLESHEET_RULES];
  }
  return [];
}

// Every step `@theme inline` publishes has to be named in lib/classNames.ts, or Tailwind Merge
// cannot disambiguate it — and an unknown value does not conflict with ANYTHING, so both
// classes survive and stylesheet order silently picks the winner. Each ladder fails its own
// way: an unnamed `--text-*` is read as a colour, so the step is DROPPED when an ink utility
// follows and IGNORED when another size does; unnamed `--leading-*` and `--radius-*` simply
// stop resolving against each other. All three have happened here.
//
// Read from `@theme inline` alone, because that block is exactly what Tailwind turns into
// utilities: `--radius-scale` is a multiplier and `--leading-markdown-*` are stylesheet
const violations = [];
let examined = 0;
for (const path of walk(SRC)) {
  const rules = rulesFor(path);
  if (rules.length === 0) continue;
  examined += 1;
  const rel = relative(SRC, path);
  const source = readFileSync(path, "utf8");
  const lines = source.split("\n");

  for (const { pattern, message, appliesTo, spansLines } of rules) {
    if (!spansLines) continue;
    if (appliesTo && !appliesTo(rel)) continue;
    for (const match of source.matchAll(pattern)) {
      const line = source.slice(0, match.index).split("\n").length;
      violations.push(`${rel}:${line}  ${match[0].replace(/\s+/g, " ")}  — ${message}`);
    }
  }

  lines.forEach((line, index) => {
    for (const { pattern, message, appliesTo, spansLines } of rules) {
      if (spansLines) continue;
      if (appliesTo && !appliesTo(rel)) continue;
      for (const match of line.matchAll(pattern)) {
        violations.push(`${rel}:${index + 1}  ${match[0]}  — ${message}`);
      }
    }
  });
}

if (violations.length > 0) {
  console.error(`check-design-tokens: ${violations.length} off-ladder value(s)\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}
// A derived vocabulary that came back empty matches nothing, and matching nothing reads as clean.
if (NAMED_EDGE_WIDTHS.length < 2) {
  console.error(
    `check-design-tokens: derived ${NAMED_EDGE_WIDTHS.length} named edge width(s) from globals.css — the edge rule is not reading its vocabulary.`,
  );
  process.exit(2);
}

// Floor, not a target: a guard that read nothing prints the same OK as one that read everything.
const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-design-tokens: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}
console.log(
  `check-design-tokens: ${examined} files read; type + leading + radius + tone + colour + depth + edge + layer + motion ladders clean`,
);
