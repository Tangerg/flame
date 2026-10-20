#!/usr/bin/env node
import { findCycles, readSourceGraph } from "./source-graph.mjs";
import { builtinContext, contextRootsOf } from "./builtin-contexts.mjs";

const { graph } = readSourceGraph();
const production = Object.fromEntries(
  Object.entries(graph).filter(([file]) => !/\.(test|spec)\.[tj]sx?$/.test(file)),
);
// Settings composes feature controls while features contribute Settings panes.
// Those distinct public contracts may point both ways at directory granularity;
// only a resolved module cycle makes their initialization/types mutually owned.
const cycles = findCycles(production).filter((cycle) => {
  const owners = new Set(cycle.map((file) => builtinContext(file)).filter(Boolean));
  return owners.size > 1;
});
if (cycles.length) {
  for (const cycle of cycles) console.error(cycle.join(" -> "));
  throw new Error("cross-feature public contracts form a resolved dependency cycle");
}
console.log(
  `[check-builtin-contexts] OK — ${contextRootsOf(graph).length} feature owners, no cross-feature module cycles.`,
);
