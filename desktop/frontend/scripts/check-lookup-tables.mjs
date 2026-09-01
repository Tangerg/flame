#!/usr/bin/env node
// Lookup tables keyed by a string this app did not choose must be Maps.
//
// TypeScript already refuses to index an inferred object literal with an open
// `string` (TS7053, "no index signature with a parameter of type 'string'"). The
// `Record<string, T>` annotation is precisely the switch that turns that refusal
// off — and once it is off, `constructor`, `toString`, `valueOf` and `__proto__`
// answer from `Object.prototype` and arrive typed as `T`.
//
// That is not theoretical. A file named `constructor` in a diff made
// `langFromPath` return the Object constructor, which threw one frame later on
// `lang.toLowerCase()` and took the whole diff view down; the MCP name regex
// admits `constructor` verbatim; and MCP servers name their own tools, which is
// what `toolCategory` and the tool-label table are indexed by. Fifteen tables
// across five layers carried the same defect, which is the point at which the
// invariant stops being something each callsite remembers.
//
// A Map has no prototype chain to inherit from, so `.get()` on an absent key is
// `undefined` and the `?? fallback` every one of these already wrote is finally
// true. Where the key IS closed — a union type — `Record<Union, T>` keeps
// TypeScript's exhaustiveness check and is left alone.

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const SRC = join(process.cwd(), "src");

// Handed whole to i18next, which owns its own lookup; no key from this app
// indexes them, and `check:locales` already governs their contents.
const EXEMPT = [join("src", "lib", "i18n", "locales")];

function sources(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) out.push(...sources(path));
    else if (/\.tsx?$/.test(path)) out.push(path);
  }
  return out;
}

// `const NAME: Record<string, …> = {` — the declaration form. The lazy `>` walks
// forward over nested generics (`Record<string, Record<A, B>>`) because the tail
// only matches at the outermost one.
const TABLE = /\b(?:const|let)\s+(\w+)\s*:\s*Record<\s*string\s*,[\s\S]*?>\s*=\s*\{/g;

const violations = [];

let examined = 0;
for (const file of sources(SRC)) {
  examined += 1;
  const rel = relative(process.cwd(), file);
  if (EXEMPT.some((prefix) => rel.startsWith(prefix))) continue;
  if (/\.(test|spec)\.tsx?$/.test(rel)) continue;

  const source = readFileSync(file, "utf8");
  // Strip comments so a table quoted in prose does not read as a declaration.
  const text = source.replace(/\/\/[^\n]*/g, "").replace(/\/\*[\s\S]*?\*\//g, "");

  for (const match of text.matchAll(TABLE)) {
    // `= {}` and `= { ...other }` are accumulators, not tables of answers: they
    // hold only what the surrounding code puts in them, and a spread copies an own
    // `__proto__` faithfully where a keyed assignment would drop it. Different
    // concern, different fix. A table declares its answers inline, so it opens with
    // a key.
    const afterBrace = text.slice(match.index + match[0].length);
    if (/^\s*(\}|\.\.\.)/.test(afterBrace)) continue;

    violations.push({ rel, name: match[1], line: text.slice(0, match.index).split("\n").length });
  }
}

if (violations.length > 0) {
  console.error(`[check-lookup-tables] Found ${violations.length} object-literal lookup table(s):`);
  for (const v of violations) console.error(`  ${v.rel}:${v.line}  ${v.name}`);
  console.error("");
  console.error("A `Record<string, T>` table is indexed by a key this app did not");
  console.error("choose, and an object answers `constructor` / `toString` /");
  console.error("`valueOf` / `__proto__` from its prototype — typed as T, so nothing");
  console.error("catches it. Use `new Map([...])` and `.get(key) ?? fallback`, or");
  console.error("narrow the key to a union type if it is genuinely closed.");
  process.exit(1);
}

// A guard that examined nothing reports the same "OK" as a guard that examined
// everything — `check-circular` did exactly that, on an empty graph, for its whole
// existence. The floor is far below today's count: it catches a broken walk or a moved
// tree, not ordinary growth.
const MIN_FILES_EXAMINED = 500;
if (examined < MIN_FILES_EXAMINED) {
  console.error(
    `[check-lookup-tables] only read ${examined} files (floor ${MIN_FILES_EXAMINED}) — the walk is broken.`,
  );
  process.exit(2);
}
console.log(
  `[check-lookup-tables] OK — ${examined} files read, no object-literal lookup tables with an open key.`,
);
