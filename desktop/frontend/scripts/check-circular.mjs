#!/usr/bin/env node
// Circular-dependency guard for the runtime module graph.
//
// Only RUNTIME cycles count. A `import type` edge is erased by the compiler, so two modules
// that only name each other's types are not a cycle in anything that executes — and the
// tree has two such pairs today, both deliberate.
//
// This ran on dependency-cruiser for that erasure, and reported "no runtime cycles" every
// single time — because it handed the tool the `src` DIRECTORY, and dependency-cruiser
// cruises nothing at all when given one: `totalCruised: 0`, an empty module list, and a
// clean bill of health for a graph it never looked at. Given globs instead it lists the
// files but still resolves almost no imports here, so the erasure it was chosen for was
// never reached either.
//
// madge resolves this project (check-layers has depended on it all along), so the graph
// comes from there and the erasure is done here: an edge is type-only when every import of
// that module in the importing file is `import type` / `export type`, and a cycle survives
// only if every one of its edges carries a value.
//
// Two floors below, because the failure this file is recovering from was silence, not a
// wrong answer: too few modules and too few edges each abort rather than pass.

import { execFileSync } from "node:child_process";
import { closeSync, openSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const MIN_MODULES = 600;
const MIN_EDGES = 1000;

// Redirect madge's JSON to a temp FILE rather than a stdout pipe: madge calls
// process.exit() before an async pipe drains, so a captured pipe is silently capped at
// 64KB once the graph outgrows it. Same reason check-layers does this.
const graphFile = join(tmpdir(), "flame-check-circular-madge.json");
let raw = "";
try {
  const fd = openSync(graphFile, "w");
  try {
    execFileSync(
      "npx",
      ["madge", "--extensions", "ts,tsx", "--ts-config", "tsconfig.json", "--json", "src/"],
      { stdio: ["ignore", fd, "inherit"] },
    );
  } catch {
    // madge exits non-zero on warnings yet still writes a full graph — let JSON.parse judge.
  } finally {
    closeSync(fd);
  }
  raw = readFileSync(graphFile, "utf8");
} finally {
  rmSync(graphFile, { force: true });
}

let graph;
try {
  graph = JSON.parse(raw);
} catch {
  console.error("[check-circular] madge did not produce valid JSON:");
  console.error(raw);
  process.exit(2);
}

const modules = Object.keys(graph);
const edgeCount = Object.values(graph).reduce((total, deps) => total + deps.length, 0);
if (modules.length < MIN_MODULES || edgeCount < MIN_EDGES) {
  console.error(
    `[check-circular] graph has ${modules.length} modules and ${edgeCount} edges ` +
      `(expected at least ${MIN_MODULES} and ${MIN_EDGES}).`,
  );
  console.error("Module resolution broke — this run proves nothing about cycles.");
  process.exit(2);
}

const sourceCache = new Map();
function sourceOf(relativePath) {
  let source = sourceCache.get(relativePath);
  if (source === undefined) {
    source = readFileSync(join(process.cwd(), "src", relativePath), "utf8");
    sourceCache.set(relativePath, source);
  }
  return source;
}

// Every `import`/`export … from` statement in `file` that resolves to `dep`, and whether
// each one carries values. madge reports paths relative to src/ without the extension, so
// the specifier is matched on its tail: a relative `./contracts` or an aliased
// `@/plugins/sdk/contracts` both end in the dep's own path.
const STATEMENT = /(?:^|\n)\s*(import|export)\s+(type\s+)?([\s\S]*?)\s*from\s*["']([^"']+)["']/g;

function carriesValue(file, dep) {
  const depPath = dep.replace(/\.[jt]sx?$/, "");
  const depTail = depPath.split("/").pop();
  let sawEdge = false;
  for (const [, , inlineType, clause, specifier] of sourceOf(file).matchAll(STATEMENT)) {
    const specifierPath = specifier.replace(/\.[jt]sx?$/, "");
    if (specifierPath.split("/").pop() !== depTail) continue;
    sawEdge = true;
    // `import type { A } from` and `import { type A, type B } from` are both erased; a
    // clause with any un-prefixed binding is not.
    if (inlineType) continue;
    const bindings = clause.replace(/^\{|\}$/g, "").split(",");
    if (bindings.some((binding) => binding.trim() !== "" && !/^type\s/.test(binding.trim()))) {
      return true;
    }
  }
  // An edge madge saw that no statement here explains — a dynamic import, or a specifier
  // shape this does not model. Treat it as a value edge: guessing "erased" would hide a
  // real cycle, and guessing "value" only costs a report to look at.
  return !sawEdge;
}

// Tarjan-free: the graph is small and cycles here are short. Walk every path, and record a
// cycle the first time a node repeats on it.
const cyclesByKey = new Map();
const inProgress = new Set();
const settled = new Set();

function walk(node, path) {
  inProgress.add(node);
  path.push(node);
  for (const dep of graph[node] ?? []) {
    if (inProgress.has(dep)) {
      const cycle = path.slice(path.indexOf(dep));
      if (cycle.every((from, index) => carriesValue(from, cycle[(index + 1) % cycle.length]))) {
        const rotations = cycle.map((_, index) => [
          ...cycle.slice(index),
          ...cycle.slice(0, index),
        ]);
        rotations.sort((left, right) => left.join("\0").localeCompare(right.join("\0")));
        cyclesByKey.set(rotations[0].join("\0"), [...rotations[0], rotations[0][0]]);
      }
    } else if (!settled.has(dep) && graph[dep] !== undefined) {
      walk(dep, path);
    }
  }
  path.pop();
  inProgress.delete(node);
  settled.add(node);
}

for (const module of modules) if (!settled.has(module)) walk(module, []);

const cycles = [...cyclesByKey.values()];
if (cycles.length > 0) {
  console.error(`[check-circular] Found ${cycles.length} runtime circular dependency(ies):`);
  for (const cycle of cycles) console.error(`  ${cycle.join(" > ")}`);
  console.error("");
  console.error(
    "Break each cycle, or make the relevant edge type-only when it carries only types.",
  );
  process.exit(1);
}

console.log(
  `[check-circular] OK — no runtime cycles across ${modules.length} modules, ${edgeCount} edges`,
);
