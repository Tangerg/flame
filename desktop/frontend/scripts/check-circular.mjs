#!/usr/bin/env node
import { findCycles, readSourceGraph } from "./source-graph.mjs";

const { values } = readSourceGraph();
const cycles = findCycles(values);
if (cycles.length) {
  console.error(`[check-circular] Found ${cycles.length} runtime circular dependency(ies):`);
  for (const cycle of cycles) console.error(`  ${cycle.join(" > ")}`);
  process.exit(1);
}
const edges = Object.values(values).reduce((total, deps) => total + deps.length, 0);
console.log(
  `[check-circular] OK — ${Object.keys(values).length} modules, ${edges} value edges, no runtime cycles.`,
);
