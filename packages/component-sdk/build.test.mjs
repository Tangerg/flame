import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import { rolldown } from "rolldown";
import { buildComponent } from "./build.mjs";

test("bundles private static and dynamic modules into an executable browser graph", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-component-graph-"));
  try {
    const input = join(directory, "entry.ts");
    await writeFile(join(directory, "package.json"), '{"type":"module"}');
    await writeFile(join(directory, "value.ts"), "export const value = 7;");
    await writeFile(join(directory, "lazy.ts"), "export const value = 9;");
    await writeFile(
      input,
      'export { value } from "./value.ts"; export const lazy = () => import("./lazy.ts");',
    );
    const output = join(directory, "output");
    await buildComponent({ input, directory: output, compiler: rolldown });
    const component = await import(pathToFileURL(join(output, "component.js")));
    assert.equal(component.value, 7);
    assert.equal((await component.lazy()).value, 9);
    assert.doesNotMatch(
      await readFile(join(output, "component.js"), "utf8"),
      /value\.ts|lazy\.ts/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

for (const dependency of ["node:fs", "fs", "missing-author-dependency"]) {
  test(`refuses the unpackaged browser dependency ${dependency}`, async () => {
    const directory = await mkdtemp(join(tmpdir(), "flame-component-refusal-"));
    try {
      const input = join(directory, "entry.ts");
      await writeFile(
        input,
        `import dependency from ${JSON.stringify(dependency)}; export default dependency;`,
      );
      await assert.rejects(
        buildComponent({
          input,
          directory: join(directory, "output"),
          compiler: rolldown,
        }),
        /could not resolve/i,
      );
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });
}
