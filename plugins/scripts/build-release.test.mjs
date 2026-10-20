import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { buildPlugin } from "./build.mjs";

test("replacement refuses to erase the author project", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-release-"));
  try {
    const author = join(root, "author");
    await mkdir(join(author, "package"), { recursive: true });
    const manifest = JSON.stringify({ name: "acme.plugin", version: "1.0.0" });
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
