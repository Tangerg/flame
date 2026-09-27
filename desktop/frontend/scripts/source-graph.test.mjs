import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, sep } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  assertSourceCoverage,
  findCycles,
  readSourceGraph,
  RUNTIME_CLIENT_EDGE,
} from "./source-graph.mjs";
import { builtinContext, isPublishedContextFile } from "./builtin-contexts.mjs";

function fixture(files, callback) {
  const root = mkdtempSync(join(tmpdir(), "flame-source-graph-"));
  try {
    for (const [path, content] of Object.entries({
      "tsconfig.json": JSON.stringify({
        compilerOptions: {
          module: "ESNext",
          moduleResolution: "Bundler",
          paths: { "@/*": ["./src/*"] },
          types: [],
        },
        include: ["src"],
      }),
      ...files,
    })) {
      mkdirSync(dirname(join(root, path)), { recursive: true });
      writeFileSync(join(root, path), content);
    }
    callback(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test("resolves exact modules and removes type edges before detecting cycles", () => {
  fixture(
    {
      "src/entry.ts":
        'import type { Shape } from "./types/shared"; import { value } from "@/values/shared"; export const entry = value; export type Result = Shape;',
      "src/types/shared.ts":
        'import { entry } from "../entry"; export interface Shape { value: typeof entry }',
      "src/values/shared.ts": "export const value = 1;",
    },
    (root) => {
      const { graph, values } = readSourceGraph(root);
      assert.deepEqual(
        new Set(graph["entry.ts"]),
        new Set(["types/shared.ts", "values/shared.ts"]),
      );
      assert.deepEqual(values["entry.ts"], ["values/shared.ts"]);
      assert.equal(findCycles(graph).length, 1);
      assert.deepEqual(findCycles(values), []);
    },
  );
});

test("includes side effects, re-exports, dynamic imports and Runtime package subpaths", () => {
  fixture(
    {
      "src/entry.ts":
        'import "./effect"; export { type Shape } from "./shape"; export const load = () => import("./lazy"); import type { RpcClient } from "@flame/runtime-contract/client/core"; export type Client = RpcClient;',
      "src/effect.ts": "export {};",
      "src/shape.ts": "export interface Shape {}",
      "src/lazy.ts": 'export * from "./entry";',
    },
    (root) => {
      const { graph, values } = readSourceGraph(root);
      assert.deepEqual(
        new Set(graph["entry.ts"]),
        new Set(["effect.ts", "shape.ts", "lazy.ts", RUNTIME_CLIENT_EDGE]),
      );
      assert.deepEqual(new Set(values["entry.ts"]), new Set(["effect.ts", "lazy.ts"]));
      assert.equal(findCycles(values).length, 1);
    },
  );
});

test("fails if a source is excluded or a local import cannot resolve", () => {
  fixture(
    {
      "tsconfig.json": '{"files":["src/entry.ts"]}',
      "src/entry.ts": "export {};",
      "src/omitted.ts": "export {};",
    },
    (root) => assert.throws(() => readSourceGraph(root), /source scan omitted/),
  );
  fixture({ "src/entry.ts": 'import "./missing";' }, (root) =>
    assert.throws(() => readSourceGraph(root), /unresolved local import/),
  );
  assert.throws(() => assertSourceCoverage(["a.ts", "b.ts"], ["a.ts"]), /b\.ts/);
});

test("flat plugins and layered plugins receive the same context boundary", () => {
  assert.equal(
    builtinContext("plugins/builtin/settings/hooks/index.ts"),
    "plugins/builtin/settings/hooks",
  );
  assert.equal(
    builtinContext("plugins/builtin/settings/hooks/adapters/runtimeHooks.ts"),
    "plugins/builtin/settings/hooks",
  );
  assert.equal(builtinContext("plugins/builtin/theme/kit/theme.ts"), "plugins/builtin/theme");
  assert.equal(
    builtinContext("plugins/builtin/observability/frontendObservability.ts"),
    "plugins/builtin/observability",
  );
  assert.equal(builtinContext("plugins/builtin/index.ts"), null);
});

test("presentation kits publish entrypoints while keeping their implementations private", () => {
  const context = "plugins/builtin/settings/kit";
  assert.equal(isPublishedContextFile(`${context}/index.ts`, context), true);
  assert.equal(isPublishedContextFile(`${context}/SettingRow.tsx`, context), false);
  assert.equal(
    isPublishedContextFile("plugins/builtin/agent/adapters/gateway.ts", "plugins/builtin/agent"),
    false,
  );
  assert.equal(
    isPublishedContextFile("plugins/builtin/agent/public/input.ts", "plugins/builtin/agent"),
    true,
  );
});

test("includes module-specific TypeScript extensions in cycle and coverage checks", () => {
  fixture(
    {
      "src/entry.ts": 'export { next } from "./next.mjs";',
      "src/next.mts": 'export { entry } from "./last.cjs"; export const next = 1;',
      "src/last.cts": 'export * from "./entry";',
    },
    (root) => {
      const { values } = readSourceGraph(root);
      assert.equal(Object.keys(values).length, 3);
      assert.equal(findCycles(values).length, 1);
    },
  );
});

test("ordinary imports used only as types do not create runtime cycles", () => {
  fixture(
    {
      "src/entry.ts": 'import { Other } from "./other"; export class Entry { value?: Other }',
      "src/other.ts": 'import { Entry } from "./entry"; export class Other { value?: Entry }',
    },
    (root) => {
      const { graph, values } = readSourceGraph(root);
      assert.equal(findCycles(graph).length, 1);
      assert.deepEqual(findCycles(values), []);
    },
  );
});

test("resolved Runtime SDK paths and aliases keep the client boundary", () => {
  const sdk = fileURLToPath(import.meta.resolve("@flame/runtime-contract/client/ids"));
  fixture(
    {
      "tsconfig.json": JSON.stringify({
        compilerOptions: {
          module: "ESNext",
          moduleResolution: "Bundler",
          paths: { "@runtime-client": [sdk] },
          types: [],
        },
        include: ["src"],
      }),
      "src/direct.ts": "",
      "src/aliased.ts": 'export { asRunId } from "@runtime-client";',
    },
    (root) => {
      const specifier = relative(join(root, "src"), sdk).split(sep).join("/").replace(/\.ts$/, "");
      writeFileSync(
        join(root, "src/direct.ts"),
        `export { asRunId } from ${JSON.stringify(specifier)};`,
      );
      const { graph, values } = readSourceGraph(root);
      for (const file of ["direct.ts", "aliased.ts"]) {
        assert.deepEqual(graph[file], [RUNTIME_CLIENT_EDGE]);
        assert.deepEqual(values[file], [RUNTIME_CLIENT_EDGE]);
      }
    },
  );
});

test("static template dynamic imports retain runtime cycles", () => {
  fixture(
    {
      "src/entry.ts": "export const load = () => import(`./other`);",
      "src/other.ts": 'import { load } from "./entry"; export const other = load;',
    },
    (root) => {
      const { graph, values } = readSourceGraph(root);
      assert.deepEqual(graph["entry.ts"], ["other.ts"]);
      assert.deepEqual(values["entry.ts"], ["other.ts"]);
      assert.equal(findCycles(values).length, 1);
    },
  );
});
