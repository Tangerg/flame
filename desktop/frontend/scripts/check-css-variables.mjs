#!/usr/bin/env node
// A `var()` with no fallback and no definition computes to nothing, and nothing is a legal
// value for every property it could have been. `--shadow-floating` was one: a name that no
// stylesheet ever defined, read by an image tray that therefore cast no shadow at all, for as
// long as it took someone to compare it against a design that said it should.
//
// The judge is the BUILT css, because that is the only place every sheet is present at once —
// a name defined in `markdown.css` and read in `globals.css` is not a violation, and reading
// either file alone cannot tell.
//
// Reads that carry a fallback are excluded on purpose: `var(--composer-overlay, 0px)` states
// its own answer for the case where nothing set it, which is the whole point of the fallback.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const ROOT = new URL("../", import.meta.url).pathname;
const DIST = join(ROOT, "dist", "assets");

/**
 * The names nothing in CSS defines because something at RUNTIME does.
 *
 * The value is the file that sets it, and the guard checks that the file still names it — so
 * this list cannot rot into a set of excuses for variables whose setter was deleted. Base UI
 * writes its own three onto the positioner element, which no file of ours can be pointed at.
 */
const BASE_UI = Symbol("set by Base UI's Positioner");
const RUNTIME_SET = new Map([
  ["--agent-overflow-distance", "src/ui/agent/overflow-label.tsx"],
  ["--agent-overflow-duration", "src/ui/agent/overflow-label.tsx"],
  ["--dock-measure", "src/plugins/builtin/shell/kernel/panel/dockWidth.ts"],
  ["--sidebar-width", "src/ui/agent/sidebar.tsx"],
  ["--anchor-width", BASE_UI],
  ["--available-height", BASE_UI],
  ["--available-width", BASE_UI],
]);

let sheets;
try {
  sheets = readdirSync(DIST).filter((name) => name.endsWith(".css"));
} catch {
  sheets = [];
}
if (sheets.length === 0) {
  console.error("[check-css-variables] no built stylesheet in dist/assets — run the build first.");
  process.exit(2);
}
const css = sheets.map((name) => readFileSync(join(DIST, name), "utf8")).join("\n");

const defined = new Set([...css.matchAll(/(--[A-Za-z0-9_-]+)\s*:/g)].map((m) => m[1]));
const bare = new Set();
for (const match of css.matchAll(/var\(\s*(--[A-Za-z0-9_-]+)\s*([,)])/g))
  if (match[2] === ")") bare.add(match[1]);

/**
 * A spec's probe reads a variable too, and is the one reader the built sheet cannot see.
 *
 * `foundation.visual.spec.ts` measured `--app-content-card-radius` by setting it on a
 * throwaway element and comparing the result — a name this repository has never defined in
 * any commit. It resolved to `0px`, so the assertion compared `0px` against `0px` and could
 * not fail, under a comment describing a corner the shell declares per visual style. A
 * component's `var()` is compiled into the sheet above and already covered; a spec's is not.
 */
function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}
const probes = new Map();
for (const path of walk(join(ROOT, "visual"))) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  const text = readFileSync(path, "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\/\/[^\n]*/g, "");
  for (const match of text.matchAll(/var\(\s*(--[A-Za-z0-9_-]+)\s*\)/g)) {
    const line = text.slice(0, match.index).split("\n").length;
    if (!probes.has(match[1])) probes.set(match[1], `${relative(ROOT, path)}:${line}`);
  }
}

const violations = [];
for (const name of [...bare].sort()) {
  if (defined.has(name)) continue;
  const setter = RUNTIME_SET.get(name);
  if (setter === undefined) {
    violations.push(`\`${name}\` is read with no fallback and nothing defines it`);
    continue;
  }
  if (setter === BASE_UI) continue;
  if (!readFileSync(join(ROOT, setter), "utf8").includes(name))
    violations.push(`\`${name}\` is listed as set by ${setter}, which no longer names it`);
}
for (const [name, where] of probes)
  if (!defined.has(name) && !RUNTIME_SET.has(name))
    violations.push(`\`${name}\` is read by ${where} and nothing defines it`);
for (const [name, setter] of RUNTIME_SET)
  if (!bare.has(name) && !defined.has(name) && !probes.has(name))
    violations.push(
      `\`${name}\` is listed as runtime-set${setter === BASE_UI ? "" : ` by ${setter}`} but nothing reads it`,
    );

if (violations.length > 0) {
  console.error(`check-css-variables: ${violations.length} variable(s) that resolve to nothing\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  console.error("");
  console.error("Define it in `globals.css`, give the read a fallback that states the answer for");
  console.error("when nothing set it, or add it above with the file that sets it at runtime.");
  process.exit(1);
}

console.log(
  `check-css-variables: ${defined.size} defined, ${bare.size} read without a fallback in the sheet and ${probes.size} in a spec; every one resolves`,
);
