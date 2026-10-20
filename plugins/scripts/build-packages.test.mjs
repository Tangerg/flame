import { test } from "node:test";
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, mkdir, writeFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./build-packages.mjs", import.meta.url));
const run = promisify(execFile);

async function withSeededOutput(fn) {
  const dir = await mkdtemp(join(tmpdir(), "flame-build-packages-"));
  const output = join(dir, "release");
  await mkdir(output, { recursive: true });
  await writeFile(join(output, "previous-release.txt"), "previous\n");
  try {
    await fn(output);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

// An invalid selection must be rejected before the output is touched, so a
// missing or non-plugin --plugin can never replace an existing release with an
// empty directory.
async function rejectsWithoutReplacing(args) {
  await withSeededOutput(async (output) => {
    await assert.rejects(
      run("node", [
        script,
        "--os",
        "darwin",
        "--arch",
        "arm64",
        "--output",
        output,
        "--replace",
        ...args,
      ]),
    );
    assert.deepEqual(await readdir(output), ["previous-release.txt"]);
  });
}

test("a missing plugin name fails without replacing output", () =>
  rejectsWithoutReplacing(["--plugin", "does-not-exist"]));

test("a non-plugin directory selection fails without replacing output", () =>
  rejectsWithoutReplacing(["--plugin", "scripts"]));

test("a missing platform fails before touching output", () =>
  withSeededOutput(async (output) => {
    await assert.rejects(
      run("node", [
        script,
        "--plugin",
        "memory",
        "--arch",
        "arm64",
        "--output",
        output,
      ]),
    );
    assert.deepEqual(await readdir(output), ["previous-release.txt"]);
  }));
