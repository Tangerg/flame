#!/usr/bin/env node
import { existsSync } from "node:fs";
import { join, resolve } from "node:path";
import { ARTIFACT_GRAPH, analyzeArtifactGraph, readArtifactGraph } from "../artifactGraph.mjs";

const directory = resolve("dist");
if (!existsSync(join(directory, ARTIFACT_GRAPH))) {
  console.error("[check-bundle-size] artifact graph not found — run `npm run build` first.");
  process.exit(2);
}

try {
  const graph = readArtifactGraph(directory);
  const result = analyzeArtifactGraph(graph);
  const bytes = (paths) => paths.reduce((sum, path) => sum + graph.files[path].bytes, 0);
  console.log(
    "[check-bundle-size] build graph: static dependencies plus explicitly declared startup edges (raw bytes)",
  );
  console.log(
    `  bootstrap-only JS: ${bytes(result.bootstrap.js)}; CSS: ${bytes(result.bootstrap.css)}`,
  );
  console.log(`  startup JS: ${bytes(result.startup.js)}; CSS: ${bytes(result.startup.css)}`);
  for (const { label, used, ceiling } of result.budgets) {
    console.log(
      `  ${label.padEnd(20)} ${used} / ${ceiling} (${((used / ceiling) * 100).toFixed(1)}%) ${used > ceiling ? "FAIL" : "OK"}`,
    );
  }
  for (const feature of result.features) {
    console.log(
      `  ${feature.label}: full ${feature.fullBytes}; startup-shared ${feature.sharedBytes}; incremental ${feature.used}`,
    );
  }
  console.log(`  HTML prefetch hints (separate from startup): ${result.prefetch.length}`);
  console.log(
    `  HTML preload hints (separate from the module/static graph): ${result.preload.length}`,
  );
  console.log(
    "  These are build-graph byte sets, not first-paint, runtime fetch or evaluation measurements.",
  );
  for (const error of result.errors) console.error(`  ${error}`);
  if (result.errors.length || result.budgets.some(({ used, ceiling }) => used > ceiling)) {
    console.error(
      "[check-bundle-size] FAIL — a declared graph boundary or byte ceiling was exceeded.",
    );
    process.exitCode = 1;
  }
} catch (error) {
  console.error(`[check-bundle-size] FAIL — ${error.message}`);
  process.exitCode = 1;
}
