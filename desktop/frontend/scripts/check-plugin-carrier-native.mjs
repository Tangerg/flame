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
  const compilerFlags = ["-tags", "production", "-ldflags", linkerFlags];
  const goOptions = {
    cwd: fileURLToPath(new URL("../..", import.meta.url)),
    timeout: 120_000,
    maxBuffer: 1024 * 1024,
  };
  await execute("go", ["test", ...compilerFlags, "./testdata/plugin-carrier"], goOptions);
  await execute(
    "go",
    ["build", ...compilerFlags, "-o", executable, "./testdata/plugin-carrier"],
    goOptions,
  );
  const { stdout } = await execute(executable, [], { timeout: 30_000, maxBuffer: 1024 * 1024 });
  const reports = stdout.split("\n").filter((value) => value.startsWith("carrier-result:"));
  assert.equal(reports.length, 1, "native fixture must publish exactly one terminal result");
  const [line] = reports;
  const report = JSON.parse(line.slice("carrier-result:".length));
  console.log(JSON.stringify(report, null, 2));
  const { carrier: result, effects } = report;
  assertCarrierIsolation(result, effects.peerPackets);
  assert.equal(result.native, true);
  assert.equal(result.hostOrigin, "wails://localhost");
  assert.equal(
    result.nativeBaseline.calls,
    1,
    "the trusted binding positive control must reach Go",
  );
  assert.ok(
    result.nativeBaseline.ready >= 1,
    "the trusted native event positive control must settle",
  );
  assert.equal(
    result.nativeBaseline.resourceRequests,
    0,
    "the trusted host must not issue plugin resource probes",
  );
  assert.equal(
    effects.resourceRequests,
    result.nativeBaseline.resourceRequests,
    "a blocked plugin request reached the native asset handler",
  );
  assert.equal(
    effects.calls,
    result.nativeBaseline.calls,
    "an opaque-origin frame reached a native binding",
  );
  assert.equal(
    effects.ready,
    result.nativeBaseline.ready,
    "an opaque-origin frame advanced native window state",
  );
} finally {
  await rm(directory, { recursive: true, force: true });
}
