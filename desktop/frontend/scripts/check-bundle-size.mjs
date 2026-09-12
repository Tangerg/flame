#!/usr/bin/env node
// Startup budget — guards the work the app performs before first paint, and the
// code splitting that keeps that work bounded.
//
// WHY RAW BYTES, NOT GZIP: this is a desktop app. Assets are read from an
// embedded filesystem through the webview's own scheme — never fetched over a
// network, never gzipped in transit. Compressed size therefore measures a cost
// we do not pay; the cost we do pay is parse + evaluate, which tracks raw
// bytes. Measuring gzip here was a web-era proxy that had drifted into
// measuring nothing: the entry budget sat at 3.5x the actual payload (a gate
// that could no longer fail) while the lazy features sat at ~90% of theirs,
// about to fail for a reason that costs a desktop user nothing.
//
// TWO DIFFERENT INVARIANTS, GUARDED DIFFERENTLY:
//   * The entry payload is startup work. Every byte is parsed before anything
//     renders, so it gets a real ceiling with modest headroom.
//   * The heavyweight lazy features (syntax highlighting, diagram rendering)
//     are read from local disk on first use, so their absolute size is not a
//     user-visible cost. What WOULD be is one of them landing on the startup
//     path. So they are guarded structurally — each must exist as its own chunk
//     and must not be referenced from index.html — with only a runaway ceiling
//     on size.
//
// Method:
//   1. Parse dist/index.html for the entry assets (<script src>,
//      <link rel="modulepreload">, stylesheets).
//   2. Sum their raw bytes, compare against BUDGETS.
//   3. For each lazy feature: assert its chunk exists, assert index.html does
//      not reference it, sum its raw bytes against a runaway ceiling.
//   4. Assert the dynamically imported KaTeX stylesheet stays out of the HTML;
//      sharing a manual chunk name with eager KaTeX JS must not make it eager.
//
// Bumping a budget: allowed, in the same commit as the growth, with a reason.
// Reviewers SHOULD push back on a bump carrying no justification — a budget
// nobody defends decays into the gate this file had already become.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { basename, join } from "node:path";

const DIST = "dist";
const INDEX_HTML = join(DIST, "index.html");

// Budgets in RAW bytes. Recorded 2026-08-11 from a clean `npm run build`:
// entry JS 2.56 MB, entry CSS 100.9 KB. JS includes modulepreloads — omitting
// them measured only the root chunk even though the webview parses its static
// dependency graph before first paint. Headroom is ~14% — loose enough that
// adding a normal feature doesn't bump it, tight enough that pulling a
// heavyweight dependency onto the startup path fails here.
// 2026-08-30: lowered from 3_000_000 after splitting OpenTelemetry per package
// took 110 KB of SDKs and exporters off the startup path. A ceiling left at the
// old value would have let that space refill without anyone noticing.
// 2026-08-31: lowered again by the 63 KB that lazy workspace-view bodies took
// off the entry, holding the same ~6% headroom.
// 2026-09-08: CSS raised for the StyleX migration's double count, and because
// 0.4% headroom no longer told a regression from a rounding difference — it
// failed on 491 bytes of ordinary work.
//
// That note called the double count TRANSIENT and predicted the ceiling would
// come back "well under the old 135 KB" once every file had migrated. The
// migration is now complete and the prediction was WRONG. Measured with the
// business layer empty of class attributes — and read the table as the dated
// snapshot it is, taken while Tailwind was still in the bundle, which is why it
// has a row for it. The counts have moved since (884 atomic rules and 2884
// occurrences of the padding on a live route, against the 821 and 2630 below);
// what has not moved is the SHAPE of the answer, which is the reason the
// ceiling sits where it does.
//
//   @layer utilities (Tailwind)   24.4 KB   18%
//   StyleX (unlayered)            57.4 KB   42%   821 atomic rules
//   @layer theme + base            9.5 KB    7%
//   globals/markdown, unlayered   ~43.8 KB   33%   :root vars, .md, keyframes
//                                 ────────
//                                  135.1 KB  (+ 2.4 KB markdown chunk)
//
// So ~49 KB of Tailwind utilities became 24 KB of them plus 57 KB of StyleX.
// The reason is measurable and structural: 23.7 KB — 41% of StyleX's output,
// 29 bytes per rule across 2630 occurrences — is the `:not(#\#)` specificity
// padding. That padding is not waste; it is the property that made the whole
// migration worth doing, because it is why no utility can silently outrank a
// design decision. It gzips to almost nothing (135 KB → 27 KB) but this ships
// in a desktop webview that reads the file locally, so the raw bytes are what
// gets parsed at every launch, which is what this budget measures.
//
// The ceiling therefore stays where the migration left it, with ~6% headroom
// over the real post-migration number rather than over a hoped-for one.
const BUDGETS = {
  js: 2_785_000,
  css: 146_000,
};

// Runaway ceilings, RAW bytes. Recorded 2026-08-04: syntax highlighting
// 1383.0 KB, diagram rendering 1542.1 KB. Deliberately ~2x current — these are
// on-demand local reads, so the ceiling exists to catch an accident of scale (a
// full language set, a second diagram engine), not to ration bytes.
const LAZY_FEATURES = [
  {
    label: "settings panes",
    prefixes: [
      "AppearancePane-",
      "ApprovalsPane-",
      "ConnectionPane-",
      "HooksPane-",
      "IconGallery-",
      "IconShowcase-",
      "McpServersPane-",
      "PersonalizationPane-",
      "PluginsPane-",
      "ProvidersPane-",
      "SchedulesPane-",
      "UsagePane-",
    ],
    ceiling: 200_000,
  },
  // Named one by one rather than pattern-matched: the list IS the coverage proof, so a
  // view added without registering it here fails as "missing chunk" rather than passing
  // quietly. What this guards is the barrel, which pulled all twenty onto startup.
  {
    label: "workspace views",
    prefixes: [
      "agent-docs-",
      "agentMemory-",
      "diff-",
      "file-",
      "inbox-",
      "knowledge-",
      "notifications-",
      "plan-",
      "recipes-",
      "search-",
      "skills-",
      "timeline-",
      "tools-",
    ],
    ceiling: 250_000,
  },
  { label: "syntax highlighting", prefix: "shiki-", ceiling: 3_000_000 },
  { label: "diagram rendering", prefix: "mermaid-", ceiling: 3_000_000 },
];

function loadIndexHtml() {
  try {
    return readFileSync(INDEX_HTML, "utf8");
  } catch (err) {
    console.error(`[check-bundle-size] ${INDEX_HTML} not found — run \`npm run build\` first.`);
    console.error(err.message);
    process.exit(2);
  }
}

function extractEntryAssets(html) {
  // Vite writes every static dependency as a modulepreload. All JS assets in
  // this document are therefore startup work, not only the root <script>.
  const js = [...new Set([...html.matchAll(/["'](\/assets\/[^"']+\.js)["']/g)].map((m) => m[1]))];
  const css = [
    ...html.matchAll(/<link[^>]*\srel="stylesheet"[^>]*\shref="(\/assets\/[^"]+\.css)"/g),
    ...html.matchAll(/<link[^>]*\shref="(\/assets\/[^"]+\.css)"[^>]*\srel="stylesheet"/g),
  ].map((m) => m[1]);
  return { js, css };
}

// Every asset index.html points at, by whatever mechanism — script, stylesheet,
// modulepreload, prefetch. This is the set a lazy feature has to stay out of:
// anything reachable from the document is startup work, and a `modulepreload`
// is exactly how a lazy chunk quietly stops being lazy.
function referencedAssets(html) {
  return new Set(
    [...html.matchAll(/["'](\/assets\/[^"']+)["']/g)].map((match) => basename(match[1])),
  );
}

function rawSizeOf(path) {
  statSync(path); // throw early with a clear message if missing
  return readFileSync(path).length;
}

function rawSizeOfAsset(relativeUrl) {
  // index.html references are absolute (/assets/...). Strip the leading slash
  // to make a filesystem path relative to dist/.
  return rawSizeOf(join(DIST, relativeUrl.replace(/^\//, "")));
}

function formatKb(bytes) {
  return `${(bytes / 1024).toFixed(1)} KB`;
}

/** Prints one budget line; returns true when it is over. */
function report(label, used, budget) {
  const pct = ((used / budget) * 100).toFixed(1);
  const over = used > budget;
  console.log(
    `  ${label.padEnd(20)} ${formatKb(used).padStart(10)} / ${formatKb(budget).padStart(10)}  (${pct}%) ${over ? "FAIL" : "OK"}`,
  );
  return over;
}

const html = loadIndexHtml();
const { js, css } = extractEntryAssets(html);

if (js.length === 0) {
  console.error("[check-bundle-size] no <script> entry found in index.html");
  process.exit(2);
}

let failed = false;

console.log("[check-bundle-size] entry payload — parsed before first paint (raw):");
for (const [extension, assets] of [
  ["js", js],
  ["css", css],
]) {
  const used = assets.reduce((sum, url) => sum + rawSizeOfAsset(url), 0);
  if (report(extension.toUpperCase(), used, BUDGETS[extension])) failed = true;
}

const assetDirectory = join(DIST, "assets");
const assetNames = readdirSync(assetDirectory);
const entryReferences = referencedAssets(html);

const mathStyles = assetNames.filter((name) => name.startsWith("katex-") && name.endsWith(".css"));
console.log("[check-bundle-size] lazy styles — must stay off the startup path (raw):");
if (mathStyles.length === 0) {
  console.error("  math stylesheet: no KaTeX CSS chunk found");
  failed = true;
} else {
  const eagerMathStyles = mathStyles.filter((name) => entryReferences.has(name));
  if (eagerMathStyles.length > 0) {
    console.error(
      `  math stylesheet: index.html references ${eagerMathStyles.join(", ")} — it must stay lazy.`,
    );
    failed = true;
  }
  report(
    "math stylesheet",
    mathStyles.reduce((sum, name) => sum + rawSizeOf(join(assetDirectory, name)), 0),
    100_000,
  );
}

console.log("[check-bundle-size] lazy features — must stay off the startup path (raw):");
for (const { label, prefix, prefixes = [prefix], ceiling } of LAZY_FEATURES) {
  const missing = prefixes.filter(
    (candidate) => !assetNames.some((name) => name.startsWith(candidate) && name.endsWith(".js")),
  );
  const chunks = assetNames.filter(
    (name) => prefixes.some((candidate) => name.startsWith(candidate)) && name.endsWith(".js"),
  );
  if (missing.length > 0) {
    console.error(
      `  ${label}: no chunk found for ${missing.join(", ")} — did it get folded into the entry?`,
    );
    failed = true;
  }
  const eager = chunks.filter((name) => entryReferences.has(name));
  if (eager.length > 0) {
    console.error(`  ${label}: index.html references ${eager.join(", ")} — it must stay lazy.`);
    failed = true;
  }
  const used = chunks.reduce((sum, name) => sum + rawSizeOf(join(assetDirectory, name)), 0);
  if (report(label, used, ceiling)) failed = true;
}

if (failed) {
  console.error("");
  console.error("[check-bundle-size] FAIL — a guarded payload exceeded its budget.");
  console.error("If this growth is intentional, update the budget in");
  console.error("scripts/check-bundle-size.mjs in the same commit, with a reason.");
  process.exit(1);
}
