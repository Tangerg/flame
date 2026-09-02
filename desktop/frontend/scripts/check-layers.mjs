#!/usr/bin/env node
// Layer-boundary guard. Complements check-circular.mjs: that one forbids
// cycles, this one forbids *upward* / cross-layer import edges that the
// clean-architecture layering disallows. Run off the same madge graph.
//
// Philosophy mirrors CLAUDE.md's "强反向不变量 (known wrong directions)":
// rather than a full allow-matrix (brittle, false-positive prone), each
// guarded layer declares the set of layers it must NEVER import. Edges to
// any other layer are allowed — so this catches the architectural
// regressions we care about (UI/plugin upward deps, rpc purity) without
// policing every legitimate inward dependency.

import { execFileSync } from "node:child_process";
import { closeSync, openSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Ordered longest-prefix-first: first match wins. Paths are relative to
// src/ (how madge reports them when invoked with `src/`).
//
// Every entry names a directory that EXISTS. Rules for a layer that was retired
// (or never built) guard nothing while reading as architecture, so the shape of
// the tree stays visible here; `UNDECLARED_ROOTS` below is what keeps a new one
// from slipping in unguarded, and it costs five lines instead of fifty.
const LAYER_PREFIXES = [
  ["foundation/", "foundation"],
  ["plugins/sdk/", "sdk"],
  ["plugins/builtin/", "builtin"],
  ["plugins/", "plugins-glue"], // Slot / PluginProvider / etc. — UI glue
  ["main/", "main"],
  ["rpc/", "rpc"],
  ["lib/", "lib"],
  // The design system is three rings, and the direction between them is the
  // whole point: Base UI lives behind `ui/primitives` (nothing else may reach
  // for it), `ui/atoms` dresses those primitives, and `ui/agent` composes atoms
  // into shell-shaped pieces. Before this split the three were one `ui` layer,
  // so the primitives boundary held by discipline alone — and an atom drifted
  // into the agent ring exactly once.
  ["ui/primitives/", "ui-primitives"],
  ["ui/atoms/", "ui-atoms"],
  ["ui/agent/", "ui-agent"],
  ["ui/", "ui"],
  ["pages/", "pages"],
];

// Roots that carry no dependency direction: assets, the test harness, and the
// bare entry files that sit beside them at src/ root.
const UNGUARDED_ROOTS = new Set(["styles", "test"]);

function layerOf(path) {
  for (const [prefix, layer] of LAYER_PREFIXES) if (path.startsWith(prefix)) return layer;
  return "other";
}

/** A src/ subtree that no rule above classifies — a new layer nobody declared a
 *  direction for. Silence there is the failure mode this whole file exists to
 *  prevent, so it is reported rather than defaulted to "unguarded". */
function undeclaredRoot(path) {
  const [root, ...rest] = path.split("/");
  if (rest.length === 0) return null; // bare entry file at src/ root
  if (root === "..") return null; // outside src/ — the runtime contract's sample fixtures
  if (UNGUARDED_ROOTS.has(root)) return null;
  return layerOf(path) === "other" ? root : null;
}

// Per guarded layer: the layers it must NEVER import. `rpc` is near-total —
// it is meant to be self-contained. The outer guards lock the upward edges.
const UI_RINGS = ["ui", "ui-primitives", "ui-atoms", "ui-agent"];
const UI = [...UI_RINGS, "pages", "builtin", "plugins-glue"];
const FORBIDDEN = {
  // Consumer-neutral scalar values. This is the innermost app layer and may
  // import only external packages or itself.
  foundation: [...UI, "main", "rpc", "sdk", "lib"],
  // Standalone protocol layer — externals + its own files only.
  rpc: [...UI, "main", "sdk", "lib"],
  // The plugin SDK is a platform layer — it must not depend on the UI it
  // is consumed by (locks the MessageContext inversion fix).
  sdk: [...UI],
  // Utility layer. `rpc` stays allowed — it's the standalone protocol layer
  // below lib (see `rpc` above, which forbids lib), so mapping an error type to
  // copy from here runs downhill. Everything else is uphill and was the hole
  // that mattered: with only `[...UI]` forbidden, lib could import the plugin
  // registry, and it did — so `ui/atoms/shiki-code-block` reached it *through*
  // `lib/highlight`, laundering the edge the `ui` rings forbid. A module in lib
  // that needs app state is not a utility; it either belongs to the context that
  // owns the state, or the value gets published down to it (`lib/appearance`).
  lib: [...UI, "sdk", "main"],
  // Local design-system layer — presentation only, never backend wiring. `sdk`
  // is forbidden too: an atom that reads the plugin registry is an atom that can
  // only be dressed by this app.
  ui: ["main", "rpc", "sdk", "plugins-glue", "builtin", "pages"],
  // Headless ring: Base UI re-exports and nothing else. It must not know about
  // the tokens, the atoms, or the shell that consume it.
  "ui-primitives": [
    "main",
    "rpc",
    "ui",
    "ui-atoms",
    "ui-agent",
    "pages",
    "builtin",
    "plugins-glue",
    "sdk",
  ],
  // Token-dressed controls. May reach primitives; must not reach the shell ring
  // above it, or anything outside the design system.
  "ui-atoms": ["main", "rpc", "ui-agent", "pages", "builtin", "plugins-glue", "sdk"],
  // Shell composites. May reach atoms and primitives; still no wiring, and no
  // reaching back into the app that mounts it.
  "ui-agent": ["main", "rpc", "pages", "builtin", "plugins-glue", "sdk"],
  // The view layer reaches the backend only through hooks — the SDK's
  // data-query hooks and selectors — never the composition root
  // (`main/container`) or the raw protocol client (`rpc`) directly.
  pages: ["main", "rpc"],
};

// A directory named any of these under plugins/builtin/<ctx>/ marks <ctx> as a
// bounded context (it has opted into the layout). `public/` is the only surface
// a foreign context may import; the rest are context-private. Contexts with no
// boundary dir (flat plugin folders like theme/ or defaults/) aren't policed.
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

// The builtin manifest is the plugin composition root: it imports every
// plugin's registration entry wherever it lives in the tree (a context holds
// several plugins, each with its own index/bootstrap), exactly as
// main/container may reach anything. It's exempt as an importer; peer contexts
// get no such license.
const BUILTIN_MANIFEST = "plugins/builtin/index.ts";
const TEST_SETUP = "test/setup.ts";
// A test sitting in the manifest's own directory tests the *assembled* plugin
// set — cross-context invariants no single context can check — so it loads
// plugins the way the manifest does. Nested test files get no such license.
const COMPOSITION_TEST = /^plugins\/builtin\/[^/]+\.test\.tsx?$/;

function atCompositionRoot(file) {
  return file === BUILTIN_MANIFEST || file === TEST_SETUP || COMPOSITION_TEST.test(file);
}

// A peer context may import only another context's `public/` facade. Any other
// cross-context import — including into a loose file sitting at the context
// root, not just the named-internal dirs — reaches a private part of the
// context and is a violation. This is strictly stronger than the old "must not
// reach application/domain/adapters/presentation/ui" rule: it also closes the
// loophole of importing a root-level file that lives in no boundary dir at all.
function crossContextViolation(file, dep, contextRoots) {
  if (atCompositionRoot(file)) return null; // composition root (manifest, its tests, test setup)
  const depContext = builtinContext(dep, contextRoots);
  if (!depContext) return null; // dep isn't inside any recognized context
  const fromContext = builtinContext(file, contextRoots);
  if (fromContext === depContext) return null; // same context — its own business
  if (dep.startsWith(`${depContext}/public/`)) return null; // the published facade
  return {
    file,
    dep,
    from: fromContext ? fromContext.replace("plugins/builtin/", "") : "outside-context",
    to: depContext.replace("plugins/builtin/", ""),
  };
}

// The rings INSIDE a context, which neither rule above sees: `check-layers`
// classifies every file under plugins/builtin/ as one layer, so the direction
// between a context's own domain / application / adapters / ui has been held by
// discipline alone. It has held — this rule was written against a tree with zero
// violations — which is exactly when to lock it, because the cost is nothing now
// and a rewrite later.
//
// Inner to outer: domain ← application ← { adapters, presentation, ui }. Each
// ring names what it must never reach for. `public/` is a re-export surface, not
// a ring, and is policed by check-published-boundaries instead.
const RING_ORDER = ["domain", "application", "presentation", "adapters", "ui"];
const RING_FORBIDS = {
  // Pure model: no orchestration, no ports, no rendering. A domain that reaches
  // for its application layer has stopped being the thing the application is
  // about.
  domain: ["application", "presentation", "adapters", "ui"],
  // Orchestration: may use the model, may NOT reach the ports that serve it (it
  // declares those) or anything that draws.
  application: ["adapters", "ui"],
  // View models: shapes for a renderer, which is why they may read the model —
  // and must not reach a port or a component.
  presentation: ["adapters", "ui"],
  // Ports and their implementations: may serve the inside, must not draw.
  adapters: ["ui"],
};

function ringOf(path, contextRoot) {
  const rest = path.slice(contextRoot.length + 1).split("/");
  // Nested plugin folders (chat/composer/...) put the ring deeper than the root.
  return rest.find((segment) => RING_ORDER.includes(segment)) ?? null;
}

function ringViolation(file, dep, contextRoots) {
  const context = builtinContext(file, contextRoots);
  if (!context || context !== builtinContext(dep, contextRoots)) return null;
  const from = ringOf(file, context);
  const to = ringOf(dep, context);
  if (!from || !to || from === to) return null;
  if (!RING_FORBIDS[from]?.includes(to)) return null;
  return { from: `${context.replace("plugins/builtin/", "")}/${from}`, to };
}

// Redirect madge's JSON to a temp FILE rather than capturing its stdout pipe.
// madge calls process.exit() before an async stdout *pipe* finishes draining
// (Node's classic exit-truncates-piped-stdout bug), so a captured pipe is
// silently capped at the 64KB buffer once the graph grows past it — which
// check:circular dodges only because `--circular` output is tiny. A file fd
// flushes synchronously on close, so the whole graph survives at any size.
const graphFile = join(tmpdir(), "flame-check-layers-madge.json");
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
    // madge can exit non-zero on warnings yet still write a full graph — read
    // whatever landed and let JSON.parse below be the judge.
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
  console.error("[check-layers] madge did not produce valid JSON:");
  console.error(raw);
  process.exit(2);
}

// An empty or truncated graph makes every rule below vacuously true, and still prints OK.
const MIN_MODULES = 600;
const MIN_EDGES = 1000;
const moduleCount = Object.keys(graph).length;
const graphEdgeCount = Object.values(graph).reduce((total, deps) => total + deps.length, 0);
if (moduleCount < MIN_MODULES || graphEdgeCount < MIN_EDGES) {
  console.error(
    `[check-layers] graph has ${moduleCount} modules and ${graphEdgeCount} edges ` +
      `(expected at least ${MIN_MODULES} and ${MIN_EDGES}).`,
  );
  console.error("Module resolution broke — this run proves nothing.");
  process.exit(2);
}

const violations = [];
const contextRoots = contextRootsOf(graph);
const undeclared = new Set();
for (const [file, deps] of Object.entries(graph)) {
  const root = undeclaredRoot(file);
  if (root) undeclared.add(root);
  // Tests may cross LAYERS to wire fixtures (e.g. loading a plugin to exercise
  // the reducer), and a test naturally reaches its own module's internals — the
  // layering invariant is about production dependency direction. The CONTEXT
  // boundary still holds for them: a test that reaches past a foreign context's
  // facade couples this context's suite to another's internals, so that context
  // can't refactor without breaking tests it doesn't own. Every test in the tree
  // already goes through `public/`; this keeps it that way.
  const isTest = /\.(test|spec)\.[tj]sx?$/.test(file);
  const from = layerOf(file);
  const forbidden = isTest ? [] : (FORBIDDEN[from] ?? []);
  for (const dep of deps) {
    const depRoot = undeclaredRoot(dep);
    if (depRoot) undeclared.add(depRoot);
    const to = layerOf(dep);
    if (forbidden.includes(to)) {
      violations.push({ file, dep, from, to });
    }
    // Tests are exempt from the LAYER rule above for fixture wiring; the ring
    // rule is about production direction too, so they are exempt here as well.
    const ringBreak = isTest ? null : ringViolation(file, dep, contextRoots);
    if (ringBreak) {
      violations.push({
        file,
        dep,
        from: `ring:${ringBreak.from}`,
        to: `ring:${ringBreak.to}`,
      });
    }
    const contextViolation = crossContextViolation(file, dep, contextRoots);
    if (contextViolation) {
      violations.push({
        file,
        dep,
        from: `context:${contextViolation.from}`,
        to: `context-private:${contextViolation.to}`,
      });
    }
  }
}

if (undeclared.size > 0) {
  console.error(
    `[check-layers] src/ subtree(s) with no declared direction: ${[...undeclared].sort().join(", ")}`,
  );
  console.error("Add each to LAYER_PREFIXES + FORBIDDEN, or to UNGUARDED_ROOTS if it carries no");
  console.error("dependency direction at all. A new layer nobody declared is an unguarded one.");
  process.exit(1);
}

if (violations.length > 0) {
  console.error(`[check-layers] Found ${violations.length} layer-boundary violation(s):`);
  for (const v of violations) {
    console.error(`  ${v.from} → ${v.to}:  ${v.file}  →  ${v.dep}`);
  }
  console.error("");
  console.error("An inner layer is importing an outer one, or a plugin context");
  console.error("is reaching past another context's public/ facade (that facade is");
  console.error("the only surface importable across contexts). Invert the dependency,");
  console.error("or route it through the public surface.");
  process.exit(1);
}

console.log("[check-layers] OK — no layer-boundary violations.");
