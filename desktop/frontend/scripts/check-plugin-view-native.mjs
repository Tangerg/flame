import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

assert.equal(process.platform, "darwin");
assert.ok(process.argv[2], "run through the desktop task graph");
const execute = promisify(execFile);
const directory = await mkdtemp(join(tmpdir(), "flame-native-page-"));
try {
  const executable = join(directory, "probe");
  await execute(
    "go",
    [
      "build",
      "-tags",
      "production,plugincarrierprobe",
      "-ldflags",
      process.argv[2],
      "-o",
      executable,
      ".",
    ],
    {
      cwd: fileURLToPath(new URL("../..", import.meta.url)),
      timeout: 120000,
      maxBuffer: 1024 * 1024,
    },
  );
  const { stdout } = await execute(executable, [], { timeout: 30000, maxBuffer: 1024 * 1024 });
  const results = stdout.split("\n").filter((value) => value.startsWith("{"));
  assert.equal(results.length, 1, "the native gate must publish exactly one terminal result");
  const result = JSON.parse(results[0]);
  console.log(result);
  assert.equal(result.error, undefined);
  assert.deepEqual(result, { Calls: 1, Ready: 1, Packets: 0, Requests: 0 });
} finally {
  await rm(directory, { recursive: true, force: true });
}
