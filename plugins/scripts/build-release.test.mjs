import protocol from "@flame/backend-sdk/protocol" with { type: "json" };
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { buildPlugin } from "./build.mjs";
import { buildBackend } from "@qimu/node/build";
import { rolldown } from "rolldown";

test("exported native author plans load without a Node backend SDK", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-native-author-"));
  try {
    const author = join(root, "native author 作者");
    await mkdir(join(author, "package"), { recursive: true });
    await writeFile(
      join(author, "package/plugin.json"),
      JSON.stringify({ name: "example.native", version: "1.0.0" }),
    );
    await writeFile(
      join(author, "package/mcp.json"),
      JSON.stringify({
        mcpServers: { native: { command: "./bin/native" } },
      }),
    );
    await cp(
      new URL("./build.mjs", import.meta.url),
      join(author, "build.mjs"),
    );
    execFileSync(
      process.execPath,
      [
        "--input-type=module",
        "--eval",
        `import assert from "node:assert/strict";
import { pathToFileURL } from "node:url";
const author = process.argv[1];
const { readBuildPlan } = await import(pathToFileURL(author + "/build.mjs"));
const plan = await readBuildPlan(author);
assert.equal(plan.native, true);
assert.deepEqual(plan.managed, []);
`,
        author,
      ],
      { cwd: author, stdio: "pipe" },
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

for (const [kind, content, problem] of [
  [
    "missing dependency",
    'import "missing-backend-package";',
    /resolve|unresolved|not found/i,
  ],
  [
    "computed import",
    "const name=process.env.MODULE; await import(name);",
    /dynamic imports must name/,
  ],
  [
    "computed require",
    "const name=process.env.MODULE; require(name);",
    /require must name/,
  ],
  ["native addon", 'import "./binding.node";', /native addons/],
])
  test(`portable backend refuses ${kind} before publication`, async () => {
    const directory = await mkdtemp(join(tmpdir(), "flame-backend-closure-"));
    try {
      const input = join(directory, "author.ts");
      await writeFile(input, content + "\nexport default {};\n");
      await assert.rejects(
        buildBackend({
          target: protocol.constants.nodeTarget,
          maximumFrameBytes: protocol.constants.maximumFrameBytes,
          input,
          directory: join(directory, "output"),
          compiler: rolldown,
        }),
        problem,
      );
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  });

test("replacement refuses to erase the author project", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-release-"));
  try {
    const author = join(root, "author");
    await mkdir(join(author, "package"), { recursive: true });
    const manifest = JSON.stringify({
      $schema: "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
      name: "acme.plugin",
      version: "1.0.0",
    });
    await writeFile(join(author, "package/plugin.json"), manifest);
    for (const output of [author, root, join(author, "package/out")]) {
      await assert.rejects(
        buildPlugin({ source: author, output, os: "darwin", arch: "arm64" }),
        /build destinations must be outside the plugin source/,
      );
    }
    assert.equal(
      await readFile(join(author, "package/plugin.json"), "utf8"),
      manifest,
    );
    const output = join(root, "output");
    await buildPlugin({ source: author, output, os: "darwin", arch: "arm64" });
    assert.equal(await readFile(join(output, "plugin.json"), "utf8"), manifest);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("repository native packaging resolves Go modules with workspace mode disabled", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-module-release-"));
  try {
    const output = join(root, "release");
    execFileSync(
      process.execPath,
      [
        fileURLToPath(new URL("./build-packages.mjs", import.meta.url)),
        "--plugin",
        "memory",
        "--os",
        "linux",
        "--arch",
        "amd64",
        "--output",
        output,
      ],
      {
        cwd: root,
        env: {
          ...process.env,
          GOWORK: "off",
          GOOS: "windows",
          GOARCH: "arm64",
        },
        stdio: "pipe",
      },
    );
    const release = JSON.parse(
      await readFile(join(output, "memory/manifest.json"), "utf8"),
    );
    assert.equal(release.package, "flame.memory");
    assert.match(release.release, /^[a-f0-9]{64}$/);
    assert.deepEqual(
      release.units.find((unit) => unit.id === "backend/memory").variants,
      [{ os: "linux", arch: "amd64", path: "bin/memory" }],
    );
    const executable = await readFile(join(output, "memory/bin/memory"));
    assert.deepEqual([...executable.subarray(0, 4)], [0x7f, 0x45, 0x4c, 0x46]);
    assert.equal(executable.readUInt16LE(18), 62);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

for (const name of ["plain-author", "author project", "作者项目", "author %#"])
  test(`exported component builder accepts ${name}`, async () => {
    const root = await mkdtemp(join(tmpdir(), "flame-author-component-"));
    try {
      const author = join(root, name);
      await mkdir(join(author, "component"), { recursive: true });
      await writeFile(
        join(author, "component/component.tsx"),
        'export const title = "author component";\n',
      );
      await mkdir(join(author, "package"));
      await writeFile(
        join(author, "package/plugin.json"),
        JSON.stringify({
          name: "example.component",
          version: "1.0.0",
          extensions: {
            "io.github.tangerg.flame": { declaration: "declaration.json" },
          },
        }),
      );
      await writeFile(
        join(author, "package/declaration.json"),
        JSON.stringify({ components: [{ export: "title" }] }),
      );
      await cp(
        new URL("./build.mjs", import.meta.url),
        join(author, "build.mjs"),
      );
      await cp(
        new URL("./author-component.mjs", import.meta.url),
        join(author, "build-component.mjs"),
      );
      await symlink(
        fileURLToPath(new URL("../../node_modules", import.meta.url)),
        join(author, "node_modules"),
        "junction",
      );
      execFileSync(process.execPath, [join(author, "build-component.mjs")], {
        cwd: root,
        stdio: "pipe",
      });
      const output = await readFile(
        join(author, "package/io.github.tangerg.flame/components/component.js"),
        "utf8",
      );
      assert.match(output, /author component/);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
