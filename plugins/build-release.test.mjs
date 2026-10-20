import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import {
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { promisify } from "node:util";

const run = promisify(execFile);

test("replacement refuses to erase the author project", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-release-"));
  try {
    const author = join(root, "author");
    await mkdir(author);
    await copyFile(
      new URL("./build.mjs", import.meta.url),
      join(author, "build.mjs"),
    );
    const manifest = JSON.stringify({ name: "acme.plugin", version: "1.0.0" });
    await writeFile(join(author, "plugin.json"), manifest);
    await assert.rejects(
      run(process.execPath, [
        join(author, "build.mjs"),
        "--os",
        "darwin",
        "--arch",
        "arm64",
        "--output",
        author,
        "--replace",
      ]),
      /build destinations must be outside the plugin source/,
    );
    assert.equal(await readFile(join(author, "plugin.json"), "utf8"), manifest);
    await readFile(join(author, "build.mjs"));
    const packages = join(root, "packages");
    await run(process.execPath, [
      join(author, "build.mjs"),
      "--os",
      "darwin",
      "--arch",
      "arm64",
      "--output",
      packages,
    ]);
    assert.equal(
      await readFile(join(packages, "acme.plugin", "plugin.json"), "utf8"),
      manifest,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
