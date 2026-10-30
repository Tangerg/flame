import { execFileSync } from "node:child_process";
import { readdir } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { stageNode } from "../../tooling/node-companion.mjs";

export async function sourceTests(directory) {
  const tests = [];
  async function visit(path) {
    let entries;
    try {
      entries = await readdir(path, { withFileTypes: true });
    } catch (error) {
      if (error.code === "ENOENT") return;
      throw error;
    }
    for (const entry of entries) {
      const child = join(path, entry.name);
      if (entry.isDirectory()) await visit(child);
      else if (
        entry.isFile() &&
        /\.(?:test|spec)\.[cm]?[jt]sx?$/.test(entry.name)
      )
        tests.push(relative(directory, child).split(sep).join("/"));
    }
  }
  for (const plugin of await readdir(directory, { withFileTypes: true })) {
    if (!plugin.isDirectory()) continue;
    for (const source of ["backend", "contracts", "component"])
      await visit(join(directory, plugin.name, source));
  }
  return tests.sort();
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const directory = fileURLToPath(new URL("../", import.meta.url));
  const tests = await sourceTests(directory);
  if (tests.length === 0) throw new Error("no plugin source tests found");
  const node = await stageNode({
    os: process.platform,
    arch: process.arch === "x64" ? "amd64" : process.arch,
    output: fileURLToPath(new URL("../../out/toolchains", import.meta.url)),
    offline: true,
  });
  execFileSync(
    node,
    [
      fileURLToPath(
        new URL("../../node_modules/vitest/vitest.mjs", import.meta.url),
      ),
      "run",
      "--no-file-parallelism",
      ...tests,
    ],
    { cwd: directory, stdio: "inherit", env: process.env },
  );
}
