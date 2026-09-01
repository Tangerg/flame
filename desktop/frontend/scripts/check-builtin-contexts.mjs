#!/usr/bin/env node
// Built-in plugin context guard. `check-layers` already blocks reaching into
// another context's application/domain/adapters/presentation internals; this
// guard watches the remaining legal seam — public-to-public context imports —
// and fails only when those public edges form a context-level cycle.

import { execFileSync } from "node:child_process";
import { closeSync, openSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const CONTEXT_BOUNDARY = new Set([
  "application",
  "presentation",
  "domain",
  "adapters",
  "public",
  "ui",
]);

function contextRootFromBoundary(path) {
  const parts = path.split("/");
  if (parts[0] !== "plugins" || parts[1] !== "builtin") return null;
  for (let i = 2; i < parts.length; i++) {
    if (CONTEXT_BOUNDARY.has(parts[i])) return i > 2 ? parts.slice(0, i).join("/") : null;
  }
  return null;
}

function contextRootsOf(graph) {
  const roots = new Set();
  for (const [file, deps] of Object.entries(graph)) {
    const fileRoot = contextRootFromBoundary(file);
    if (fileRoot) roots.add(fileRoot);
    for (const dep of deps) {
      const depRoot = contextRootFromBoundary(dep);
      if (depRoot) roots.add(depRoot);
    }
  }
  return [...roots].sort((a, b) => b.length - a.length);
}

function builtinContext(path, contextRoots) {
  if (!path.startsWith("plugins/builtin/")) return null;
  return contextRoots.find((root) => path === root || path.startsWith(`${root}/`)) ?? null;
}

function isPublicContextFile(path, context) {
  return path.startsWith(`${context}/public/`);
}

function readMadgeGraph() {
  const graphFile = join(tmpdir(), "flame-check-builtin-contexts-madge.json");
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
      // madge can exit non-zero on warnings yet still write a full graph.
    } finally {
      closeSync(fd);
    }
    raw = readFileSync(graphFile, "utf8");
  } finally {
    rmSync(graphFile, { force: true });
  }
  try {
    return JSON.parse(raw);
  } catch {
    console.error("[check-builtin-contexts] madge did not produce valid JSON:");
    console.error(raw);
    process.exit(2);
  }
}

function contextName(root) {
  return root.replace("plugins/builtin/", "");
}

function findCycles(edges) {
  const cycles = [];
  const stack = [];
  const inStack = new Set();
  const visited = new Set();

  function visit(node) {
    if (inStack.has(node)) {
      cycles.push(stack.slice(stack.indexOf(node)).concat(node));
      return;
    }
    if (visited.has(node)) return;
    visited.add(node);
    inStack.add(node);
    stack.push(node);
    for (const next of edges.get(node) ?? []) visit(next);
    stack.pop();
    inStack.delete(node);
  }

  for (const node of edges.keys()) visit(node);
  return cycles;
}

const graph = readMadgeGraph();
// The same floor `check-circular` carries, for the same reason and off the same graph: an
// empty or truncated madge result makes every rule below vacuously true, and the run still
// prints OK. This proves module resolution worked before any conclusion is drawn.
const MIN_MODULES = 600;
const MIN_EDGES = 1000;
const moduleCount = Object.keys(graph).length;
const graphEdgeCount = Object.values(graph).reduce((total, deps) => total + deps.length, 0);
if (moduleCount < MIN_MODULES || graphEdgeCount < MIN_EDGES) {
  console.error(
    `[check-builtin-contexts] graph has ${moduleCount} modules and ${graphEdgeCount} edges ` +
      `(expected at least ${MIN_MODULES} and ${MIN_EDGES}).`,
  );
  console.error("Module resolution broke — this run proves nothing.");
  process.exit(2);
}

const contextRoots = contextRootsOf(graph);
const edges = new Map();
const edgeFiles = new Map();

for (const [file, deps] of Object.entries(graph)) {
  if (/\.(test|spec)\.[tj]sx?$/.test(file)) continue;
  const from = builtinContext(file, contextRoots);
  if (!from) continue;
  for (const dep of deps) {
    const to = builtinContext(dep, contextRoots);
    if (!to || to === from || !isPublicContextFile(dep, to)) continue;
    if (!edges.has(from)) edges.set(from, new Set());
    edges.get(from).add(to);
    edgeFiles.set(`${from}→${to}`, `${file} → ${dep}`);
  }
}

// A context must not CONTAIN a context. When it does, the two share a directory prefix and
// read as one place, while every guard here treats them as strangers: `workspace` could not
// touch `workspace/events`, and `workspace/session-navigation`'s own test could not reach
// the dock store it existed to drive. Either the outer folder is a context or it is a
// namespace of them — `chat` and `settings` show the namespace shape, `agent` and
// `workspace` the context shape.
const nested = [];
for (const outer of contextRoots) {
  for (const inner of contextRoots) {
    if (inner !== outer && inner.startsWith(`${outer}/`)) nested.push([outer, inner]);
  }
}
if (nested.length > 0) {
  console.error(`[check-builtin-contexts] Found ${nested.length} nested context(s):`);
  for (const [outer, inner] of nested) {
    console.error(`  ${contextName(outer)} contains ${contextName(inner)}`);
  }
  console.error("");
  console.error("Flatten the inner context into the outer one's rings, or move the outer's");
  console.error("own rings into a named sub-context so the folder is a namespace only.");
  process.exit(1);
}

const cycles = findCycles(edges);
if (cycles.length > 0) {
  console.error(`[check-builtin-contexts] Found ${cycles.length} public context cycle(s):`);
  for (const cycle of cycles) {
    console.error("  " + cycle.map(contextName).join(" -> "));
    for (let i = 0; i < cycle.length - 1; i++) {
      const detail = edgeFiles.get(`${cycle[i]}→${cycle[i + 1]}`);
      if (detail) console.error(`    ${detail}`);
    }
  }
  console.error("");
  console.error("Invert one edge through an extension point or move the shared concept");
  console.error("to a lower public abstraction so bounded contexts stay acyclic.");
  process.exit(1);
}

const edgeCount = [...edges.values()].reduce((sum, set) => sum + set.size, 0);
console.log(`[check-builtin-contexts] OK — ${edgeCount} public context edge(s), no cycles.`);
