#!/usr/bin/env node
// Every class a source file writes by hand must be one our own stylesheets define.
//
// This replaces a guard that asked the opposite question — does the BUILT css emit a rule for
// this utility — and that question stopped being answerable the moment the product had no
// utility framework left. Its test for "is this string a class list at all" was "at least two
// of its tokens resolve", so a string where NONE resolved read as prose and was skipped. The
// rail that positions the transcript's turn marks is twelve utilities in one constant; it went
// dead in the same commit that removed Tailwind and the guard reported every class rendering.
//
// The new question has no such hole: the set of legal hand-written class names is small,
// closed, and ours. Anything else is either a utility from a framework that is gone or a
// typo, and both render nothing.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const STYLES = join(ROOT, "src", "styles");

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}

const defined = new Set();
const sheets = walk(STYLES).filter((path) => path.endsWith(".css"));
for (const path of sheets) {
  const css = readFileSync(path, "utf8").replace(/\\/g, "");
  for (const match of css.matchAll(/\.(-?[A-Za-z][A-Za-z0-9_-]*)/g)) defined.add(match[1]);
}
if (defined.size === 0) {
  console.error("[check-authored-classes] no class defined in src/styles — the read is broken.");
  process.exit(2);
}

/** Comments hold the names a refactor RETIRED, quoted so the next reader knows what moved. */
const uncomment = (text) => text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");

/** A class-name position: the prop, any `*ClassName` prop, or the class joiner itself. */
const POSITION = /(?:[A-Za-z]*[cC]lassName\s*[=:]\s*\{?|\bcn\(|\bclsx\()\s*(["'`])([^"'`\n]*)\1/g;
const NAMED = /[A-Za-z]*[cC]lassName\s*[=:]\s*\{?\s*([A-Z][A-Z0-9_]*)\b/g;

const violations = [];
let examined = 0;
for (const path of [join(ROOT, "src"), join(ROOT, "visual")].flatMap(walk)) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  if (/\.(test|spec)\.tsx?$/.test(path)) continue;
  examined += 1;
  const rel = relative(ROOT, path);
  const text = uncomment(readFileSync(path, "utf8"));

  const report = (literal, index) => {
    for (const token of literal
      .replace(/\$\{[^}]*\}/g, " ")
      .split(/\s+/)
      .filter(Boolean)) {
      if (defined.has(token)) continue;
      const line = text.slice(0, index).split("\n").length;
      violations.push(`${rel}:${line} \`${token}\` — no stylesheet defines this class`);
    }
  };

  for (const match of text.matchAll(POSITION)) report(match[2], match.index);
  // A constant hides the string from the position above, which is exactly where the last
  // twelve dead utilities were living.
  for (const match of text.matchAll(NAMED)) {
    const decl = new RegExp(`const\\s+${match[1]}\\s*(?::[^=]*)?=\\s*\\n?\\s*"([^"]*)"`).exec(text);
    if (decl) report(decl[1], match.index);
  }
}

if (violations.length > 0) {
  console.error(`check-authored-classes: ${violations.length} class(es) that style nothing\n`);
  for (const violation of [...new Set(violations)]) console.error(`  ${violation}`);
  console.error("");
  console.error("A component states its own style with `stylex.create`. A class name is only for");
  console.error("what StyleX cannot express — a descendant rule or a global — and globals.css");
  console.error("has to define it.");
  process.exit(1);
}

const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-authored-classes: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}

console.log(
  `check-authored-classes: ${examined} files read against ${defined.size} defined classes; every authored class resolves`,
);
