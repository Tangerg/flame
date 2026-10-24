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
