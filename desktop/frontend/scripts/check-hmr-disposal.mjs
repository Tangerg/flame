// A module-scope listener survives its own module. Vite's HMR replaces a module's exports in
// place and does NOT undo the side effects it already ran, so a `subscribe` at import time keeps
// firing through a stale closure and every reload stacks another one. CLAUDE.md §5 requires
// `disposeOnHmr` for exactly this, and nothing checked it.
//
// The failure mode is why: the app is correct in production, correct on a fresh dev boot, and
// gets slower the longer someone works in it — no error, no failing test, nothing a reviewer can
// see in a diff. By the time it is noticeable, the cause is thirty reloads back.
//
// Module scope is read as "the line starts in column 0". Under this repo's formatter that is
// exactly the set of calls that run at import; a registration inside a function or a callback is
// indented, runs when its owner runs, and is that owner's to clean up.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

const REGISTRATION = /(?:\.subscribe\(|\baddEventListener\(|\bsetInterval\()/;
const DISPOSAL = "disposeOnHmr";

function registrations(source) {
  return source
    .split("\n")
    .map((line, index) => ({ line, number: index + 1 }))
    .filter(({ line }) => line.length > 0 && !/^\s/.test(line) && REGISTRATION.test(line));
}

// The guard proves its own rule every run rather than trusting that a pattern which matched once
// still matches. A detector that quietly stops firing reads exactly like a clean tree.
const SELF_TEST = [
  { name: "bare module-scope subscribe", source: "const u = store.subscribe(fn);\n", flagged: 1 },
  {
    name: "module-scope subscribe with disposal",
    source: "const u = store.subscribe(fn);\ndisposeOnHmr(u);\n",
    flagged: 0,
  },
  {
    name: "registration inside a function",
    source: "function activate() {\n  store.subscribe(fn);\n}\n",
    flagged: 0,
  },
  { name: "module-scope listener", source: 'window.addEventListener("x", fn);\n', flagged: 1 },
];

for (const { name, source, flagged } of SELF_TEST) {
  const found = source.includes(DISPOSAL) ? 0 : registrations(source).length;
  if (found !== flagged) {
    console.error(
      `check-hmr-disposal: the detector no longer works — "${name}" reported ${found}, expected ${flagged}`,
    );
    process.exit(2);
  }
}

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

const violations = [];
let examined = 0;
let registered = 0;
for (const path of walk(SRC)) {
  if (![".ts", ".tsx"].includes(extname(path))) continue;
  const rel = relative(SRC, path);
  if (rel.includes(".test.") || rel === "lib/hmr.ts") continue;
  examined += 1;

  const source = readFileSync(path, "utf8");
  const found = registrations(source);
  if (found.length === 0) continue;
  registered += found.length;
  if (source.includes(DISPOSAL)) continue;
  for (const { line, number } of found) {
    violations.push(`${rel}:${number}  ${line.trim().slice(0, 72)}`);
  }
}

if (violations.length > 0) {
  console.error(
    `check-hmr-disposal: ${violations.length} module-scope registration(s) that outlive their module\n`,
  );
  for (const violation of violations) console.error(`  ${violation}`);
  console.error("\n  pair each with `disposeOnHmr(...)` from `@/lib/hmr` (CLAUDE.md §5).");
  process.exit(1);
}

const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `check-hmr-disposal: only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}

console.log(
  `check-hmr-disposal: ${examined} files read; ${registered} module-scope registration(s), all disposed on reload`,
);
