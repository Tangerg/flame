#!/usr/bin/env node
// A shorthand and one of its own longhands in the same StyleX style is decided by the bundler.
//
// `stylex.props(a, b)` resolves precedence between styles that name the same KEY: the later
// one wins and the loser is never emitted. `padding` and `paddingTop` are two keys, so StyleX
// emits a class for each, both land on the element at the same specificity, and which one
// applies is whichever rule the sheet happens to carry second. Nothing in the source says so,
// and the value that loses reads exactly like the value that wins.
//
// Four of these were live: a settings blurb whose top margin was `0` or `6px`, an image tray,
// a lightbox and an icon gallery. Three of the four wrote the shorthand FIRST, which reads as
// "and then override the top" — a sentence CSS understands and StyleX cannot.
//
// The cure is to write the longhands: `paddingInline` beside `paddingTop` and `paddingBottom`
// says the same thing in keys that cannot collide.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;

/** Only the families a style object in this product actually reaches for. */
const LONGHANDS_OF = {
  margin: ["Top", "Right", "Bottom", "Left", "Inline", "Block"].flatMap((side) => [
    `margin${side}`,
    `margin${side}Start`,
    `margin${side}End`,
  ]),
  padding: ["Top", "Right", "Bottom", "Left", "Inline", "Block"].flatMap((side) => [
    `padding${side}`,
    `padding${side}Start`,
    `padding${side}End`,
  ]),
  border: [
    "borderWidth",
    "borderStyle",
    "borderColor",
    "borderTopWidth",
    "borderBottomWidth",
    "borderLeftWidth",
    "borderRightWidth",
    "borderInlineWidth",
    "borderBlockWidth",
  ],
  borderRadius: [
    "borderTopLeftRadius",
    "borderTopRightRadius",
    "borderBottomLeftRadius",
    "borderBottomRightRadius",
    "borderStartStartRadius",
    "borderStartEndRadius",
    "borderEndStartRadius",
    "borderEndEndRadius",
  ],
  overflow: ["overflowX", "overflowY"],
  inset: ["top", "right", "bottom", "left", "insetInline", "insetBlock"],
  background: ["backgroundColor", "backgroundImage", "backgroundSize", "backgroundPosition"],
  font: ["fontSize", "fontFamily", "fontWeight", "fontStyle", "lineHeight"],
  flex: ["flexGrow", "flexShrink", "flexBasis"],
  gap: ["rowGap", "columnGap"],
  transition: [
    "transitionProperty",
    "transitionDuration",
    "transitionTimingFunction",
    "transitionDelay",
  ],
  animation: ["animationName", "animationDuration", "animationTimingFunction"],
  outline: ["outlineWidth", "outlineStyle", "outlineColor"],
  placeItems: ["alignItems", "justifyItems"],
  textWrap: ["textWrapMode", "textWrapStyle"],
};

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}

const violations = [];
let examined = 0;
for (const path of [join(ROOT, "src"), join(ROOT, "visual")].flatMap(walk)) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  examined += 1;
  const rel = relative(ROOT, path);
  const text = readFileSync(path, "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/[^\n]*/g, "");

  for (const opened of text.matchAll(/([A-Za-z0-9_$]+)\s*:\s*\{/g)) {
    let depth = 0;
    const from = opened.index + opened[0].length - 1;
    let to = from;
    for (; to < text.length; to++) {
      if (text[to] === "{") depth += 1;
      else if (text[to] === "}" && --depth === 0) break;
    }
    const keys = new Set(
      [...text.slice(from, to).matchAll(/(?:^|[{,\s])([a-zA-Z][a-zA-Z0-9]*)\s*:/g)].map(
        (m) => m[1],
      ),
    );
    for (const [shorthand, longhands] of Object.entries(LONGHANDS_OF)) {
      if (!keys.has(shorthand)) continue;
      const clashing = longhands.filter((longhand) => keys.has(longhand));
      if (clashing.length === 0) continue;
      const line = text.slice(0, opened.index).split("\n").length;
      violations.push(
        `${rel}:${line} \`${opened[1]}\` declares \`${shorthand}\` beside \`${clashing.join("`, `")}\``,
      );
    }
  }
}

if (violations.length > 0) {
  console.error(
    `check-shorthand-longhand: ${violations.length} style(s) whose value the bundler decides\n`,
  );
  for (const violation of violations) console.error(`  ${violation}`);
  console.error("");
  console.error("Write the longhands. `paddingInline` beside `paddingTop` says the same thing in");
  console.error("keys StyleX can resolve; a shorthand and its longhand are two keys it cannot.");
  process.exit(1);
}

const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-shorthand-longhand: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}

console.log(
  `check-shorthand-longhand: ${examined} files read; no style names a shorthand and its own longhand`,
);
