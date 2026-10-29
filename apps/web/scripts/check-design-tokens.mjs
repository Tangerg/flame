#!/usr/bin/env node
// A literal at a consuming style opts that element out of the user's theme,
// size or shape preferences. Missing steps belong to the corresponding owner.

import { assertSourceCoverage, sourceFiles } from "../../../tooling/source-graph.mjs";
import { presentationOutputs } from "../../../tooling/presentation.mjs";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const SHARED = fileURLToPath(new URL("../../../packages/ui/src", import.meta.url));
const SRC = fileURLToPath(new URL("../src", import.meta.url));
const REPOSITORY = fileURLToPath(new URL("../../../", import.meta.url));

// Only exact owner projections may declare generated values; a filename alone
// cannot exempt an independently edited stylesheet from the token contract.
const GENERATED_STYLESHEETS = new Map(
  [...(await presentationOutputs(REPOSITORY))].filter(([path]) => extname(path) === ".css"),
);
for (const [path, content] of GENERATED_STYLESHEETS) {
  if (readFileSync(join(REPOSITORY, path), "utf8") !== content) {
    throw new Error(`generated token declarations differ from their owner: ${path}`);
  }
}

// Deriving the vocabulary keeps token renames and value changes visible to this guard.
const TOKEN_DECLARATIONS = [
  readFileSync(new URL("../src/styles/globals.css", import.meta.url), "utf8"),
  ...GENERATED_STYLESHEETS.values(),
].join("\n");
const NAMED_EDGE_WIDTHS = [
  ...new Set(
    [...TOKEN_DECLARATIONS.matchAll(/^\s*--(?:control-edge|hairline)-width:\s*([\d.]+px);/gm)].map(
      (match) => match[1],
    ),
  ),
];

const MARKUP_RULES = [
  {
    pattern: /fontSize:\s*"[\d.]+(?:px|rem)"/g,
    message: "arbitrary font size — compose a `type.*` step from `tokens.stylex.ts`",
  },
  {
    // Unit line height sizes a glyph box without choosing a reading-ladder step.
    pattern: /lineHeight:\s*(?!1\s*[,}])[\d.]+\s*[,}]/g,
    message: "arbitrary line height — use a `leading.*` step",
  },
  {
    pattern: /boxShadow:\s*"(?!none"|var\()/g,
    message: "hardcoded shadow — define a `--shadow-*` token in globals.css and use it",
  },
  {
    pattern: /borderRadius:\s*"[\d.]+(?:px|rem)"/g,
    message: "arbitrary corner radius — use a `radius.*` step",
  },
  {
    // Single digits order local siblings; window-level ordering belongs to named layers.
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
    // Whole-source matching catches durations in multiline transition objects.
    pattern: /transition\s*[=:]\s*\{\{?[^}]*\bduration:\s*[\d.]+/g,
    spansLines: true,
    message: "literal animation duration — use a preset from `lib/motion`, or add the rung there",
    appliesTo: (rel) => rel !== "lib/motion.ts",
  },
  {
    // Only named widths follow the style preference; one-off strokes need no duplicate token.
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
    // A type step must carry size, tracking and leading together.
    pattern: /fontSize:\s*"var\(--(?:text|fs)-[^)]*\)"/g,
    message:
      "half a type step — compose `type.uiMd` / `type.displayMd` from `tokens.stylex.ts`, which carries the tracking and leading with it",
    appliesTo: (rel) => !rel.endsWith("tokens.stylex.ts"),
  },
  {
    pattern: /color-mix\([^)]*var\(--(?:color-(?:text|accent)|tone-\w+)\)[^)]*\)/g,
    message:
      "hand-mixed token alpha — use a ladder step (`surface.*` / `surface.field*` / `surface.<tone>Wash`)",
    // The theme kit is where the ladder's values are authored — mixing is its job.
    appliesTo: (rel) => !rel.startsWith("features/theme/"),
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
    pattern: /#(?:[\da-fA-F]{3,4}|[\da-fA-F]{6}|[\da-fA-F]{8})\b/g,
    message: "literal colour — define a token at its theme owner and use `var(--color-*)`",
  },
];

// A z-index declaration consumes a layer even in a token-owning stylesheet.
const EVERY_STYLESHEET_RULES = [
  {
    pattern: /z-index:\s*-?\d\d+/g,
    message: "unnamed stacking rung — declare a `--layer-*` step and use `var(--layer-*)`",
  },
];

const TOKEN_STYLESHEETS = new Set([
  "styles/globals.css",
  ...[...GENERATED_STYLESHEETS.keys()].map((path) => relative(SRC, join(REPOSITORY, path))),
]);

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
    return path === join(SHARED, "globals.css") || TOKEN_STYLESHEETS.has(relative(SRC, path))
      ? EVERY_STYLESHEET_RULES
      : [...EVERY_STYLESHEET_RULES, ...STYLESHEET_RULES];
  }
  return [];
}

const violations = [];
let examined = 0;
const scanned = [...walk(SRC), ...walk(SHARED)];
assertSourceCoverage(sourceFiles(SRC), scanned);
for (const path of scanned) {
  const rules = rulesFor(path);
  if (rules.length === 0) continue;
  examined += 1;
  const rel = path.startsWith(SHARED) ? `@flame/ui/${relative(SHARED, path)}` : relative(SRC, path);
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
    `check-design-tokens: derived ${NAMED_EDGE_WIDTHS.length} named edge width(s) from token owners — the edge rule is not reading its vocabulary.`,
  );
  process.exit(2);
}

console.log(
  `check-design-tokens: ${examined} files read; type + leading + radius + tone + colour + depth + edge + layer + motion ladders clean`,
);
