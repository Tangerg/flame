import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { pathToFileURL } from "node:url";
import { rolldown } from "rolldown";
import { buildComponent } from "./build.mjs";
import profile from "./runtime-profile.generated.json" with { type: "json" };

test("published components preserve the Host's Runtime error and exact JSON instances", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-component-runtime-"));
  try {
    await writeFile(join(directory, "package.json"), '{"type":"module"}');
    const runtimeEntry = createRequire(import.meta.url).resolve(
      "@flame/runtime-client",
    );
    const jsonDirectory = join(directory, "node_modules/@qimu/sdk");
    await mkdir(jsonDirectory, { recursive: true });
    await buildComponent({
      input: createRequire(import.meta.url).resolve("@qimu/sdk/json"),
      directory: jsonDirectory,
      compiler: rolldown,
    });
    await writeFile(
      join(jsonDirectory, "package.json"),
      JSON.stringify({
        type: "module",
        exports: { "./json": "./component.js" },
      }),
    );
    const hostSource = join(directory, "host.ts");
    await writeFile(
      hostSource,
      `export * from ${JSON.stringify(runtimeEntry)};`,
    );
    const hostDirectory = join(directory, "node_modules/@flame/runtime-client");
    await mkdir(hostDirectory, { recursive: true });
    await buildComponent({
      input: hostSource,
      directory: hostDirectory,
      compiler: rolldown,
    });
    await writeFile(
      join(hostDirectory, "package.json"),
      JSON.stringify({
        type: "module",
        exports: { ".": "./component.js" },
      }),
    );
    const input = join(directory, "entry.ts");
    await writeFile(
      input,
      `
        import { mutationSettlementIsUnknown, OperationFailure } from "@flame/runtime-client";
        import { isJSONNumber, parseJSON, stringifyJSON } from "@qimu/sdk/json";
        export function inspect(unknown, failure, value) {
          const source = stringifyJSON({ value });
          return {
            unknown: mutationSettlementIsUnknown(unknown),
            failure: failure instanceof OperationFailure,
            exact: isJSONNumber(value),
            source,
            decoded: parseJSON(source).value,
          };
        }
      `,
    );
    const output = join(directory, "output");
    await buildComponent({ input, directory: output, compiler: rolldown });
    const host = await import(
      pathToFileURL(join(hostDirectory, "component.js"))
    );
    const hostJSON = await import(
      pathToFileURL(join(jsonDirectory, "component.js"))
    );
    const component = await import(pathToFileURL(join(output, "component.js")));
    const result = component.inspect(
      new host.MutationLookupError(new Error("reply unavailable")),
      new host.OperationFailure("not_found", "owner refused"),
      new hostJSON.ExactNumber("1.00"),
    );
    assert.equal(result.unknown, true);
    assert.equal(result.failure, true);
    assert.equal(result.exact, true);
    assert.equal(result.source, '{"value":1.00}');
    assert.equal(result.decoded instanceof hostJSON.ExactNumber, true);
    assert.equal(result.decoded.text, "1.00");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

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
    const emitted = await buildComponent({
      input,
      directory: output,
      compiler: rolldown,
    });
    assert.deepEqual([...emitted.exports].sort(), ["lazy", "value"]);
    const graph = JSON.parse(
      await readFile(join(output, profile.moduleGraph.filename), "utf8"),
    );
    assert.deepEqual(graph, emitted.graph);
    assert.equal(graph.schemaVersion, profile.moduleGraph.schemaVersion);
    assert.equal(graph.entry, "component.js");
    assert.deepEqual(graph.sharedSpecifiers, []);
    assert.equal(graph.modules.length, 2);
    const root = graph.modules.find((module) => module.path === graph.entry);
    assert.equal(root.dynamicImports.length, 1);
    assert(
      graph.modules.some((module) => module.path === root.dynamicImports[0]),
    );
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

test("refuses a computed dynamic import before publishing an incomplete module graph", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-computed-import-"));
  try {
    const input = join(directory, "entry.ts");
    await writeFile(input, "export const load = (name) => import(name);");
    await assert.rejects(
      buildComponent({
        input,
        directory: join(directory, "output"),
        compiler: rolldown,
      }),
      /component dynamic imports must name a build-time module/,
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
