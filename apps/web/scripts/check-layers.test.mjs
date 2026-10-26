import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
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

test("flat plugins and layered plugins receive the same context boundary", () => {
  assert.equal(builtinContext("features/hooks/index.ts"), "features/hooks");
  assert.equal(builtinContext("features/hooks/adapters/runtimeHooks.ts"), "features/hooks");
  assert.equal(builtinContext("features/theme/kit/theme.ts"), "features/theme");
  assert.equal(
    builtinContext("features/observability/frontendObservability.ts"),
    "features/observability",
  );
  assert.equal(builtinContext("features/index.ts"), null);
});

test("features publish contracts while keeping their implementations private", () => {
  const context = "features/settings";
  assert.equal(isPublishedContextFile(`${context}/public/panes.ts`, context), true);
  assert.equal(isPublishedContextFile(`${context}/ui/AppearancePane.tsx`, context), false);
  assert.equal(
    isPublishedContextFile("features/agent/adapters/gateway.ts", "features/agent"),
    false,
  );
  assert.equal(isPublishedContextFile("features/agent/public/input.ts", "features/agent"), true);
});

test("keeps pure JSON external without weakening layer guards", () => {
  const jsonEntry = fileURLToPath(import.meta.resolve("@flame/runtime-client/json"));
  const jsonSource = 'export { parseJSON } from "@runtime-json";';
  fixture(
    {
      "tsconfig.json": JSON.stringify({
        compilerOptions: {
          module: "ESNext",
          moduleResolution: "Bundler",
          paths: { "@runtime-json": [jsonEntry] },
          types: [],
        },
        include: ["src"],
      }),
      "src/foundation/data.ts": jsonSource,
      "src/foundation/direct.ts": 'export { parseJSON } from "@flame/runtime-client/json";',
    },
    (root) => {
      const check = () =>
        spawnSync(
          process.execPath,
          [fileURLToPath(new URL("./check-layers.mjs", import.meta.url))],
          {
            cwd: root,
            encoding: "utf8",
          },
        );
      const neutral = check();
      assert.equal(neutral.status, 0, neutral.stderr);

      writeFileSync(
        join(root, "src/foundation/data.ts"),
        'export { createFlameClient } from "@flame/runtime-client";',
      );
      const runtime = check();
      assert.equal(runtime.status, 1);
      assert.match(runtime.stderr, /foundation → rpc/);

      writeFileSync(join(root, "src/foundation/data.ts"), jsonSource);
      mkdirSync(join(root, "src/new-layer"));
      writeFileSync(join(root, "src/new-layer/entry.ts"), "export {};");
      const undeclared = check();
      assert.equal(undeclared.status, 1);
      assert.match(undeclared.stderr, /no declared direction: new-layer/);
    },
  );
});
