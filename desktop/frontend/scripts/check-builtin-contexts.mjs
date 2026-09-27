#!/usr/bin/env node
// Built-in plugin context guard. `check-layers` already blocks reaching into
// another context's application/domain/adapters/presentation internals; this
// guard watches the remaining legal seam — public-to-public context imports —
// and fails only when those public edges form a context-level cycle.

import { readSourceGraph } from "./source-graph.mjs";
import { builtinContext, contextRootsOf, isPublishedContextFile } from "./builtin-contexts.mjs";

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

const { graph } = readSourceGraph();
const contextRoots = contextRootsOf(graph);
const edges = new Map();
const edgeFiles = new Map();

for (const [file, deps] of Object.entries(graph)) {
  if (/\.(test|spec)\.[tj]sx?$/.test(file)) continue;
  const from = builtinContext(file, contextRoots);
  if (!from) continue;
  for (const dep of deps) {
    const to = builtinContext(dep, contextRoots);
    if (!to || to === from || !isPublishedContextFile(dep, to)) continue;
    if (!edges.has(from)) edges.set(from, new Set());
    edges.get(from).add(to);
    edgeFiles.set(`${from}→${to}`, `${file} → ${dep}`);
  }
}

// A context must not CONTAIN a context: they share a directory prefix and read as one
// place, while every rule here treats them as strangers. A folder is either a context
// (`agent`, `workspace`) or a namespace of them (`chat`, `settings`), never both.
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
