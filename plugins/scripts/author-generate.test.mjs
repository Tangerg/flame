import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import {
  access,
  cp,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";
import { fileURLToPath, pathToFileURL } from "node:url";
import { generateWorkspaceContracts } from "./generate-contracts.mjs";

const run = promisify(execFile);

async function writeProject(directory, declaration = {}) {
  await mkdir(join(directory, "package"), { recursive: true });
  await writeFile(
    join(directory, "package/plugin.json"),
    JSON.stringify({
      name: "test.presentation",
      version: "1.0.0",
      extensions: {
        "io.github.tangerg.flame": { declaration: "declaration.json" },
      },
    }),
  );
  await writeFile(
    join(directory, "package/declaration.json"),
    JSON.stringify({
      managedBackends: [
        { id: "presentation", runtime: "node", entry: "backend/start.mjs" },
      ],
      ...declaration,
    }),
  );
}

async function authorProject(declaration, verify) {
  const directory = await mkdtemp(join(tmpdir(), "flame-author-generate-"));
  try {
    await writeProject(directory, declaration);
    for (const [source, target] of [
      ["author-generate.mjs", "generate.mjs"],
      ["build.mjs", "build.mjs"],
      ["contracts.mjs", "contracts.mjs"],
    ])
      await cp(new URL(source, import.meta.url), join(directory, target));
    await verify(directory);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

const contribution = (contract = "contracts/capability.generated.json") => ({
  id: "feature",
  server: "presentation",
  export: "feature",
  contract,
});

async function source(directory, name, contents) {
  await mkdir(join(directory, "contracts"), { recursive: true });
  await writeFile(join(directory, "contracts", name), contents);
}

async function compiler(directory) {
  await symlink(
    fileURLToPath(new URL("../../node_modules", import.meta.url)),
    join(directory, "node_modules"),
    "junction",
  );
}

async function developmentTool(directory, fail = false) {
  const tool = join(directory, "development-tool.mjs");
  await writeFile(
    tool,
    `import { appendFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
const args = process.argv.slice(2);
const option = name => args[args.indexOf(name) + 1];
if (args[0] !== "contract" || !args.includes("--feature") || !args.includes("--out"))
  throw new Error("invalid development tool invocation");
const feature = await readFile(option("--feature"), "utf8");
await writeFile("observed-feature.json", feature);
await writeFile("observed-args.json", JSON.stringify(args));
await appendFile("calls.txt", "called\\n");
const output = option("--out");
await mkdir(output, { recursive: true });
await writeFile(join(output, "capability.generated.json"), feature);
${fail ? "process.exit(2);" : ""}
await writeFile(join(output, "schema.json"), "{}\\n");
await writeFile(join(output, "methods.generated.json"), "[]\\n");
await writeFile(join(output, "API_REFERENCE.md"), "generated reference\\n");
if (args.includes("--ts")) {
  await mkdir(option("--ts"), { recursive: true });
  await writeFile(join(option("--ts"), "methods.generated.ts"), "export {};\\n");
}
`,
  );
  return { command: process.execPath, args: [tool], cwd: directory };
}

async function generate(directory, tool) {
  const { generateContracts } = await import(
    pathToFileURL(join(directory, "contracts.mjs")).href
  );
  return generateContracts(directory, tool);
}

async function noTemporaryFiles(directory) {
  assert.deepEqual(
    (await readdir(directory)).filter((name) =>
      name.startsWith(".flame-contract-"),
    ),
    [],
  );
}

const typedDescriptor = `
import { z } from "zod";
import { PROTOCOL, type CapabilityDescriptor } from "@qimu/node";
import { ExactNumber } from "@qimu/sdk/json";
const descriptor: CapabilityDescriptor = {
  protocol: PROTOCOL,
  reference: "test.exact.v1",
  composition: "collection",
  scope: "installation",
  methods: [{
    name: "test.exact/read.v1", kind: "query", target: "installation",
    description: "Read an exactly bounded number.",
    input: { type: "object", properties: { total: {
      type: "integer", minimum: 9007199254740993n,
      maximum: new ExactNumber("18446744073709551615"),
    } }, required: ["total"], additionalProperties: false },
    output: z.toJSONSchema(z.boolean(), { target: "draft-2020-12" }),
  }],
};
export default descriptor;
`;

test("tool-only author generation needs no feature contract or generator", () =>
  authorProject({}, async (directory) => {
    await assert.rejects(access(join(directory, "node_modules")), {
      code: "ENOENT",
    });
    const result = await run(process.execPath, [
      join(directory, "generate.mjs"),
    ]);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
  }));

test("declared author contributions still require their contract generator", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    await mkdir(join(directory, "contracts"));
    await writeFile(join(directory, "contracts/capability.json"), "{}");
    await assert.rejects(
      run(process.execPath, [join(directory, "generate.mjs")]),
      (error) =>
        /tools[/\\]qimu(?:\.exe)?/.test(error.stderr) &&
        /ENOENT/.test(error.stderr),
    );
    await noTemporaryFiles(directory);
  }));

test("TypeScript owner generation retains exact numbers through the public codec", () =>
  authorProject(
    { contributions: [contribution()], components: [{ id: "feature" }] },
    async (directory) => {
      await compiler(directory);
      await source(directory, "capability.ts", typedDescriptor);
      await generate(directory, await developmentTool(directory));
      const observed = await readFile(
        join(directory, "observed-feature.json"),
        "utf8",
      );
      assert.match(observed, /"minimum":9007199254740993[,}]/);
      assert.match(observed, /"maximum":18446744073709551615[,}]/);
      assert.match(
        observed,
        /"output":\{"\$schema":"https:\/\/json-schema.org\/draft\/2020-12\/schema","type":"boolean"\}/,
      );
      assert.equal(
        await readFile(
          join(directory, "package/contracts/capability.generated.json"),
          "utf8",
        ),
        observed,
      );
      assert.equal(
        await readFile(
          join(directory, "component/methods.generated.ts"),
          "utf8",
        ),
        "export {};\n",
      );
      assert.equal(
        await readFile(join(directory, "contracts/capability.ts"), "utf8"),
        typedDescriptor,
      );
      await assert.rejects(
        access(join(directory, "contracts/capability.json")),
        { code: "ENOENT" },
      );
      await noTemporaryFiles(directory);
    },
  ));

test("JSON owners are passed byte-for-byte without compiler dependencies", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    const original =
      '{\n  "maximum": 18446744073709551615, "name": "original"\n}\n';
    await source(directory, "capability.json", original);
    await generate(directory, await developmentTool(directory));
    assert.equal(
      await readFile(join(directory, "observed-feature.json"), "utf8"),
      original,
    );
    assert.equal(
      await readFile(join(directory, "contracts/capability.json"), "utf8"),
      original,
    );
    await assert.rejects(access(join(directory, "node_modules")), {
      code: "ENOENT",
    });
    await noTemporaryFiles(directory);
  }));

test("JSON and TypeScript cannot both own the same descriptor", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    await source(directory, "capability.json", "{}");
    await source(directory, "capability.ts", "export default {};");
    await assert.rejects(
      generate(directory, { command: "unused" }),
      /exactly one JSON or TypeScript source/,
    );
    await noTemporaryFiles(directory);
  }));

test("a declared managed contribution cannot silently omit its source", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    await assert.rejects(
      generate(directory, { command: "unused" }),
      /exactly one JSON or TypeScript source/,
    );
    await noTemporaryFiles(directory);
  }));

test("compiler failure removes temporary output and preserves published resources", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    await compiler(directory);
    await source(directory, "capability.ts", "export default { unfinished:");
    await mkdir(join(directory, "package/contracts"));
    await writeFile(
      join(directory, "package/contracts/capability.generated.json"),
      "original",
    );
    await assert.rejects(
      generate(directory, await developmentTool(directory)),
      /PARSE_ERROR/,
    );
    assert.equal(
      await readFile(
        join(directory, "package/contracts/capability.generated.json"),
        "utf8",
      ),
      "original",
    );
    await assert.rejects(access(join(directory, "observed-feature.json")), {
      code: "ENOENT",
    });
    await noTemporaryFiles(directory);
  }));

test("tool failure cannot publish partially generated resources", () =>
  authorProject(
    { contributions: [contribution()], components: [{ id: "feature" }] },
    async (directory) => {
      await source(directory, "capability.json", "{}");
      await mkdir(join(directory, "package/contracts"));
      await mkdir(join(directory, "component"));
      await writeFile(
        join(directory, "package/contracts/capability.generated.json"),
        "original",
      );
      await writeFile(
        join(directory, "component/methods.generated.ts"),
        "original component",
      );
      await assert.rejects(
        generate(directory, await developmentTool(directory, true)),
        { status: 2 },
      );
      assert.equal(
        await readFile(
          join(directory, "package/contracts/capability.generated.json"),
          "utf8",
        ),
        "original",
      );
      assert.equal(
        await readFile(
          join(directory, "component/methods.generated.ts"),
          "utf8",
        ),
        "original component",
      );
      await noTemporaryFiles(directory);
    },
  ));

test("invalid TypeScript default exports are refused before generation", () =>
  authorProject({ contributions: [contribution()] }, async (directory) => {
    await compiler(directory);
    await source(directory, "capability.ts", "export default [];");
    await assert.rejects(
      generate(directory, await developmentTool(directory)),
      /descriptor as default/,
    );
    await assert.rejects(access(join(directory, "observed-feature.json")), {
      code: "ENOENT",
    });
    await noTemporaryFiles(directory);
  }));

test("root generation follows declared names and leaves native contract owners to their generator", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-workspace-generate-"));
  try {
    const managed = join(directory, "plugins/managed");
    const native = join(directory, "plugins/native");
    const toolOnly = join(directory, "plugins/tool-only");
    await compiler(directory);
    await writeProject(managed, {
      contributions: [contribution("published/control.generated.json")],
    });
    await source(managed, "control.ts", typedDescriptor);
    await writeProject(native, {
      managedBackends: [],
      contributions: [{ ...contribution(), server: "native" }],
    });
    await writeFile(
      join(native, "package/mcp.json"),
      JSON.stringify({ mcpServers: { native: { command: "./bin/native" } } }),
    );
    await writeProject(toolOnly);
    await mkdir(join(directory, "plugins/scripts"));
    await generateWorkspaceContracts(
      directory,
      await developmentTool(directory),
    );
    const observed = await readFile(
      join(directory, "observed-feature.json"),
      "utf8",
    );
    assert.equal(
      await readFile(
        join(managed, "package/published/control.generated.json"),
        "utf8",
      ),
      observed,
    );
    await assert.rejects(
      access(join(managed, "package/published/capability.generated.json")),
      { code: "ENOENT" },
    );
    assert.equal(
      await readFile(join(directory, "calls.txt"), "utf8"),
      "called\n",
    );
    for (const project of [managed, native, toolOnly])
      await noTemporaryFiles(project);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("contributions sharing one contract have one generation owner", () =>
  authorProject(
    { contributions: [contribution(), { ...contribution(), id: "second" }] },
    async (directory) => {
      await source(directory, "capability.json", "{}");
      await generate(directory, await developmentTool(directory));
      assert.equal(
        await readFile(join(directory, "calls.txt"), "utf8"),
        "called\n",
      );
      await noTemporaryFiles(directory);
    },
  ));

test("independent descriptors cannot overwrite shared schema and method projections", () =>
  authorProject(
    {
      contributions: [
        contribution(),
        contribution("contracts/second.generated.json"),
      ],
    },
    async (directory) => {
      await assert.rejects(
        generate(directory, { command: "unused" }),
        /separate projection directories/,
      );
      await noTemporaryFiles(directory);
    },
  ));

test("component method projections require a single descriptor owner", () =>
  authorProject(
    {
      contributions: [
        contribution(),
        contribution("other/second.generated.json"),
      ],
      components: [{ id: "feature" }],
    },
    async (directory) => {
      await assert.rejects(
        generate(directory, { command: "unused" }),
        /one contract projection owner/,
      );
      await noTemporaryFiles(directory);
    },
  ));

test("generation validates declared owners and package-relative resources", async () => {
  for (const [value, expected] of [
    [{ ...contribution(), server: "absent" }, /declared backend owner/],
    [contribution("../outside.json"), /package-relative resource path/],
    [contribution("contracts/feature.ts"), /JSON resource/],
  ]) {
    await authorProject({ contributions: [value] }, async (directory) => {
      await assert.rejects(
        generate(directory, { command: "unused" }),
        expected,
      );
      await noTemporaryFiles(directory);
    });
  }
});

test("a descriptor resource cannot replace an emitted schema projection", () =>
  authorProject(
    { contributions: [contribution("contracts/schema.json")] },
    async (directory) => {
      await source(directory, "schema.json", "{}");
      await assert.rejects(
        generate(directory, await developmentTool(directory)),
        /collides with another generated projection/,
      );
      await assert.rejects(
        access(join(directory, "package/contracts/schema.json")),
        { code: "ENOENT" },
      );
      await noTemporaryFiles(directory);
    },
  ));
