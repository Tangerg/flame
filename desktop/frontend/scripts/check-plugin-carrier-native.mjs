import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { assertCarrierIsolation } from "./plugin-carrier-assertions.mjs";

if (process.platform !== "darwin") throw new Error("the native carrier gate requires macOS");
const execute = promisify(execFile);
const linkerFlags = process.argv[2];
assert.ok(linkerFlags, "run the native carrier gate through the desktop task graph");
const directory = await mkdtemp(join(tmpdir(), "flame-plugin-carrier-"));
try {
  const executable = join(directory, "carrier-probe");
  await execute(
    "go",
    [
      "build",
      "-tags",
      "production",
      "-ldflags",
      linkerFlags,
      "-o",
      executable,
      "./testdata/plugin-carrier",
    ],
    {
      cwd: fileURLToPath(new URL("../..", import.meta.url)),
      timeout: 120_000,
      maxBuffer: 1024 * 1024,
    },
  );
  const { stdout } = await execute(executable, [], { timeout: 30_000, maxBuffer: 1024 * 1024 });
  const line = stdout.split("\n").find((value) => value.startsWith("carrier-result:"));
  assert.ok(line, "native fixture exited without a carrier result");
  const result = JSON.parse(line.slice("carrier-result:".length));
  console.log(JSON.stringify(result, null, 2));
  assertCarrierIsolation(result);
  assert.equal(result.native, true);
  assert.equal(result.hostOrigin, "wails://localhost");
  assert.equal(result.nativeBefore.calls, 1, "the trusted binding positive control must reach Go");
  assert.ok(
    result.nativeBefore.ready >= 1,
    "the trusted native event positive control must settle",
  );
  assert.equal(
    result.nativeFinal.calls,
    result.nativeBefore.calls,
    "an opaque-origin frame reached a native binding",
  );
  assert.equal(
    result.nativeFinal.ready,
    result.nativeBefore.ready,
    "an opaque-origin frame advanced native window state",
  );
} finally {
  await rm(directory, { recursive: true, force: true });
}
