import assert from "node:assert/strict";
import { readBuildPlan } from "./build.mjs";
import { rolldown } from "rolldown";
import { fileURLToPath } from "node:url";
import { buildComponent } from "@flame/component-sdk/build";

const project = fileURLToPath(new URL("./", import.meta.url));
const { declaration } = await readBuildPlan(project);
const built = await buildComponent({
  compiler: rolldown,
  input: fileURLToPath(new URL("./component/component.tsx", import.meta.url)),
  directory: fileURLToPath(
    new URL("./package/io.github.tangerg.flame/components", import.meta.url),
  ),
});

assert.deepEqual(
  [...built.exports].sort(),
  [
    ...new Set((declaration.components ?? []).map((entry) => entry.export)),
  ].sort(),
  "component exports must match declared contributions",
);
