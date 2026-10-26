import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { rolldown } from "rolldown";
import { fileURLToPath } from "node:url";
import { buildComponent } from "@flame/component-sdk/build";

const declaration = JSON.parse(
  await readFile(
    new URL(
      "../package/io.github.tangerg.flame/declaration.json",
      import.meta.url,
    ),
    "utf8",
  ),
);
const built = await buildComponent({
  compiler: rolldown,
  input: fileURLToPath(new URL("board.tsx", import.meta.url)),
  directory: fileURLToPath(
    new URL("../package/io.github.tangerg.flame/components", import.meta.url),
  ),
});

assert.deepEqual(
  [...built.exports].sort(),
  [...new Set(declaration.components.map((entry) => entry.export))].sort(),
  "component exports must match declared contributions",
);
