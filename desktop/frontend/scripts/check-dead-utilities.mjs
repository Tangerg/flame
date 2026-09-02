#!/usr/bin/env node
// A utility naming a token the theme never defined emits NOTHING, silently: `rounded-control`
// left a hover wash square, `ring-focus` drew a currentColor ring. The judge is the BUILT CSS
// rather than a list of token names, which would be a second copy of the theme and drift the
// same way. Reads `dist/`, so it runs after the build in `check:bundle`.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const SRC = join(ROOT, "src");
const DIST = join(ROOT, "dist", "assets");

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}

let stylesheets;
try {
  stylesheets = walk(DIST).filter((path) => path.endsWith(".css"));
} catch {
  stylesheets = [];
}
if (stylesheets.length === 0) {
  console.error("[check-dead-utilities] no built stylesheet in dist/assets — run the build first.");
  process.exit(2);
}
const css = stylesheets.map((path) => readFileSync(path, "utf8")).join("\n");

/** A utility is present if the built CSS names it in a selector. Selectors escape every
 *  character that is not a CSS identifier — `.px-1\.5`, `.hover\:bg-hover` — so the escapes
 *  come out first and the bare token is matched wherever a selector can introduce one. */
const selectors = css.replace(/\\/g, "");
const emitted = new Set();
for (const match of selectors.matchAll(/[.:](-?(?:[a-z0-9]+)(?:-[a-z0-9.]+)+)/g))
  emitted.add(match[1]);

/** `hover:`, `group-hover/x:`, `@sm:`, `data-[open]:`, `!`, and a leading `-` for negatives. */
function baseUtility(token) {
  let rest = token;
  while (true) {
    const colon = rest.indexOf(":");
    if (colon === -1) break;
    // An arbitrary variant carries its own brackets; anything inside them is not a prefix.
    const head = rest.slice(0, colon);
    if (head.includes("[") && !head.includes("]")) break;
    rest = rest.slice(colon + 1);
  }
  // The leading `-` STAYS: Tailwind emits `.-mx-1\\.5` for the negative and `.mx-1.5` only if
  // something asks for the positive, so stripping it would look up a utility nobody wrote.
  rest = rest.replace(/^!+/, "");
  const slash = rest.indexOf("/");
  if (slash !== -1) rest = rest.slice(0, slash);
  return rest;
}

const UTILITY = /^-?[a-z][a-z0-9]*(?:-[a-z0-9.]+)+$/;

const violations = [];
let examined = 0;
for (const path of walk(SRC)) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  if (/\.(test|spec)\.tsx?$/.test(path)) continue;
  examined += 1;
  const rel = relative(ROOT, path);
  const text = readFileSync(path, "utf8");

  for (const [, literal] of text.matchAll(/"([^"\n]*)"/g)) {
    if (!literal.includes(" ")) continue;
    const tokens = literal.split(/\s+/).filter(Boolean).map(baseUtility);
    const candidates = tokens.filter((token) => UTILITY.test(token) && !token.includes("["));
    const known = candidates.filter((token) => emitted.has(token));
    // Two resolved utilities is what makes a string a class list rather than prose that
    // happens to contain a hyphen. A tool name or an i18n key resolves none.
    if (known.length < 2) continue;
    for (const token of candidates) {
      if (emitted.has(token)) continue;
      const line = text.slice(0, text.indexOf(literal)).split("\n").length;
      violations.push(
        `${rel}:${line} \`${token}\` emits no rule — the theme defines no such token`,
      );
    }
  }
}

if (violations.length > 0) {
  console.error(`check-dead-utilities: ${violations.length} class(es) that render nothing\n`);
  for (const violation of [...new Set(violations)]) console.error(`  ${violation}`);
  console.error("");
  console.error("Either the token belongs in the ladder in globals.css, or the call site meant");
  console.error("one that is already there.");
  process.exit(1);
}

const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-dead-utilities: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}

console.log(
  `check-dead-utilities: ${examined} files read against ${emitted.size} emitted utilities; every class renders`,
);
