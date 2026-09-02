#!/usr/bin/env node
// The other half of `check-dead-utilities`: that one fails a class the source names and the
// stylesheet never emits, this one a class the stylesheet emits and the source never names.
// Both are invisible — the rule simply never meets an element.
//
// Narrow on purpose: only a selector STARTING with one of this app's classes must be applied
// by this app. A library's class is styled under a scope the app does apply
// (`.md .markdown-alert`), so it is not checked.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const SRC = join(ROOT, "src");
const STYLES = join(SRC, "styles");
const VISUAL = join(ROOT, "visual");

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}

const consumers = [...walk(SRC), ...walk(VISUAL)]
  .filter((path) => [".ts", ".tsx"].includes(extname(path)) && !path.startsWith(STYLES))
  .map((path) => readFileSync(path, "utf8"));
consumers.push(readFileSync(join(ROOT, "index.html"), "utf8"));
const applied = consumers.join("\n");

/** A class name appears in a class list, a `querySelector`, or a test locator — all of which
 *  put it next to a quote, a space, or a dot. Matching the bare token would let `.md-table`
 *  be satisfied by `md-table-scroller`. */
function isNamed(cls) {
  return new RegExp(String.raw`(?<![\w-])${cls}(?![\w-])`).test(applied);
}

const SELECTOR = /^\.([a-z][a-z0-9-]{2,})(?=[\s,:{.[])/;

const violations = [];
let examined = 0;
for (const path of walk(STYLES)) {
  if (extname(path) !== ".css") continue;
  examined += 1;
  const rel = relative(ROOT, path);
  for (const [index, line] of readFileSync(path, "utf8").split("\n").entries()) {
    const match = SELECTOR.exec(line.trim());
    if (!match || isNamed(match[1])) continue;
    violations.push(`${rel}:${index + 1} \`.${match[1]}\` is styled and never applied`);
  }
}

if (violations.length > 0) {
  console.error(`check-dead-styles: ${violations.length} rule(s) that can never match\n`);
  for (const violation of [...new Set(violations)]) console.error(`  ${violation}`);
  console.error("");
  console.error("Either something should be wearing the class, or the rule outlived what did.");
  process.exit(1);
}

if (examined === 0) {
  console.error("check-dead-styles: read no stylesheets — the walk is broken.");
  process.exit(2);
}

console.log(
  `check-dead-styles: ${examined} stylesheet(s) read; every app class reaches an element`,
);
