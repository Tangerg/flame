import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { cp, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";

const run = promisify(execFile);

async function authorProject(declaration, verify) {
  const directory = await mkdtemp(join(tmpdir(), "flame-author-generate-"));
  try {
    await mkdir(join(directory, "package"));
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
    for (const [source, target] of [
      ["author-generate.mjs", "generate.mjs"],
      ["build.mjs", "build.mjs"],
    ])
      await cp(new URL(source, import.meta.url), join(directory, target));
    await verify(directory);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

test("tool-only author generation needs no feature contract or generator", () =>
  authorProject({}, async (directory) => {
    const result = await run(process.execPath, [
      join(directory, "generate.mjs"),
    ]);
    assert.equal(result.stdout, "");
    assert.equal(result.stderr, "");
  }));

test("declared author contributions still require their contract generator", () =>
  authorProject(
    { contributions: [{ contract: "contracts/capability.generated.json" }] },
    async (directory) => {
      await assert.rejects(
        run(process.execPath, [join(directory, "generate.mjs")]),
        (error) =>
          /flame-sdk/.test(error.stderr) && /ENOENT/.test(error.stderr),
      );
    },
  ));
