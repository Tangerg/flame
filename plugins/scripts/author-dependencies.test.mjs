import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { authorDependencies } from "./author-dependencies.mjs";

async function project(files, verify) {
  const source = await mkdtemp(join(tmpdir(), "flame-author-dependencies-"));
  try {
    for (const [path, code] of Object.entries(files)) {
      const target = join(source, path);
      await mkdir(dirname(target), { recursive: true });
      await writeFile(target, code);
    }
    await verify(source);
  } finally {
    await rm(source, { recursive: true, force: true });
  }
}

const lock = {
  packages: Object.fromEntries(
    [
      ["@openuidev/lang-core", "1.0.1"],
      ["@openuidev/react-lang", "1.0.1"],
      ["react", "19.3.1"],
      ["react-dom", "19.3.1"],
      ["motion", "14.6.0"],
      ["marked", "18.1.1"],
      ["@js-temporal/polyfill", "0.5.1"],
      ["@modelcontextprotocol/sdk", "1.32.2"],
      ["happy-dom", "21.0.4"],
      ["jsdom", "1.0.0"],
      ["@edge-runtime/vm", "1.0.0"],
      ["vitest-environment-custom", "1.0.0"],
    ].map(([name, version]) => [`node_modules/${name}`, { version }]),
  ),
};
const manifest = {
  dependencies: {
    "@openuidev/lang-core": "^1.0.0",
    "@openuidev/react-lang": "^1.0.0",
    "@flame/component-sdk": "*",
    react: "^19.3.0",
    marked: "^18.0.0",
    "@js-temporal/polyfill": "^0.5.0",
  },
  devDependencies: { "@modelcontextprotocol/sdk": "1.32.2" },
};

test("author production imports select their locked packages and JSX runtime", () =>
  project(
    {
      "backend/start.ts": 'import { readFile } from "node:fs/promises";',
      "contracts/document.ts":
        'export type Node = import("@openuidev/lang-core").Node;',
      "component/component.tsx":
        'import { Renderer } from "@openuidev/react-lang"; export const View = () => <Renderer/>;',
      "backend/document.test.ts":
        'import "@modelcontextprotocol/sdk/client/index.js";',
      "component/methods.generated.ts": 'import "generated-only-package";',
      "contracts/description.ts":
        'export const example = \'import "example-only-package";\'; // import "comment-only-package";',
    },
    async (source) => {
      const result = await authorDependencies({
        source,
        manifest,
        lock,
        packages: [],
        required: [],
      });
      assert.deepEqual(result.dependencies, {
        "@openuidev/lang-core": "1.0.1",
        "@openuidev/react-lang": "1.0.1",
        react: "19.3.1",
      });
      assert.deepEqual(result.devDependencies, {
        "@modelcontextprotocol/sdk": "1.32.2",
      });
    },
  ));

test("author SDK subpath imports include packed dependency and peer closure", () =>
  project(
    {
      "component/component.ts":
        'import type { ComponentProps } from "@flame/component-sdk"; export const builder = import("@flame/component-sdk/build");',
    },
    async (source) => {
      const packages = [
        {
          manifest: {
            name: "@flame/component-sdk",
            peerDependencies: {
              "@flame/ui": "*",
              "react-dom": "*",
              "optional-host": "*",
            },
            peerDependenciesMeta: { "optional-host": { optional: true } },
          },
        },
        {
          manifest: {
            name: "@flame/ui",
            dependencies: { "@flame/icons": "*" },
            peerDependencies: { motion: "*" },
          },
        },
        {
          manifest: { name: "@flame/icons", peerDependencies: { react: "*" } },
        },
        { manifest: { name: "@flame/backend-sdk" } },
      ];
      const result = await authorDependencies({
        source,
        manifest,
        lock,
        packages,
        required: [],
      });
      assert.deepEqual(
        result.packages.map((value) => value.manifest.name),
        ["@flame/icons", "@flame/ui", "@flame/component-sdk"],
      );
      assert.deepEqual(result.dependencies, {
        react: "19.3.1",
        motion: "14.6.0",
        "react-dom": "19.3.1",
      });
    },
  ));

test("author production imports must be declared even when the lock contains them", () =>
  project(
    {
      "contracts/document.ts":
        'export { jsonToOpenUI } from "@openuidev/lang-core";',
    },
    async (source) => {
      await assert.rejects(
        authorDependencies({
          source,
          manifest: {},
          lock,
          packages: [],
          required: [],
        }),
        /undeclared dependency: @openuidev\/lang-core/,
      );
    },
  ));

test("author imports fail instead of guessing an unselected package version", () =>
  project(
    { "contracts/document.ts": 'import "@openuidev/lang-core";' },
    async (source) => {
      await assert.rejects(
        authorDependencies({
          source,
          manifest,
          lock: { packages: {} },
          packages: [],
          required: [],
        }),
        /no packaged selection/,
      );
    },
  ));

test("author test dependencies cannot rely on undeclared transitive packages", () =>
  project(
    {
      "backend/backend.test.ts":
        'import "@modelcontextprotocol/sdk/client/index.js";',
    },
    async (source) => {
      await assert.rejects(
        authorDependencies({
          source,
          manifest: { dependencies: manifest.dependencies },
          lock,
          packages: [],
          required: [],
        }),
        /author test requires undeclared dependency/,
      );
    },
  ));

for (const [description, directive, dependency, version] of [
  ["line comment", "// @vitest-environment happy-dom", "happy-dom", "21.0.4"],
  ["docblock", "/**\n * @vitest-environment jsdom\n */", "jsdom", "1.0.0"],
  ["Jest comment", "// @jest-environment happy-dom", "happy-dom", "21.0.4"],
  [
    "edge provider",
    "// @vitest-environment edge-runtime",
    "@edge-runtime/vm",
    "1.0.0",
  ],
  [
    "custom provider",
    "// @vitest-environment custom",
    "vitest-environment-custom",
    "1.0.0",
  ],
]) {
  test(`author test ${description} selects its declared locked environment`, () =>
    project(
      { "contracts/document.test.ts": `${directive}\nexport {};` },
      async (source) => {
        const result = await authorDependencies({
          source,
          manifest: {
            ...manifest,
            devDependencies: { ...manifest.devDependencies, [dependency]: "*" },
          },
          lock,
          packages: [],
          required: [],
        });
        assert.deepEqual(result.dependencies, {});
        assert.deepEqual(result.devDependencies, { [dependency]: version });
      },
    ));
}

test("author environments use the first control comment and ignore literal examples and production comments", () =>
  project(
    {
      "contracts/document.ts": "// @vitest-environment happy-dom\nexport {};",
      "contracts/node.test.ts":
        "// @vitest-environment node\n// @vitest-environment happy-dom\nexport {};",
      "contracts/example.test.ts":
        'const example = "@vitest-environment happy-dom";',
      "contracts/options.test.ts":
        '// @vitest-environment-options {"url":"https://example.test"}\nexport {};',
    },
    async (source) => {
      const result = await authorDependencies({
        source,
        manifest,
        lock,
        packages: [],
        required: [],
      });
      assert.deepEqual(result.dependencies, {});
      assert.deepEqual(result.devDependencies, {});
    },
  ));

test("author environment providers require both an explicit declaration and a locked selection", () =>
  project(
    {
      "contracts/document.test.ts":
        "// @vitest-environment happy-dom\nexport {};",
    },
    async (source) => {
      await assert.rejects(
        authorDependencies({
          source,
          manifest,
          lock,
          packages: [],
          required: [],
        }),
        /author test requires undeclared dependency: happy-dom/,
      );
      await assert.rejects(
        authorDependencies({
          source,
          manifest: {
            ...manifest,
            devDependencies: { "happy-dom": "^21.0.4" },
          },
          lock: { packages: {} },
          packages: [],
          required: [],
        }),
        /root lock has no packaged selection for happy-dom/,
      );
    },
  ));
