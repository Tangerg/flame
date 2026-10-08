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
const mode = process.argv[3];
assert.ok(linkerFlags, "run the native carrier gate through the desktop task graph");
assert.ok(mode === "wails" || mode === "isolated-webkit", "select a native carrier explicitly");

async function observe(executable, carrier) {
  const { stdout } = await execute(executable, [carrier], {
    timeout: 30_000,
    maxBuffer: 1024 * 1024,
  });
  const reports = stdout.split("\n").filter((value) => value.startsWith("carrier-result:"));
  assert.equal(reports.length, 1, "native fixture must publish exactly one terminal result");
  const report = JSON.parse(reports[0].slice("carrier-result:".length));
  console.log(JSON.stringify({ mode: carrier, ...report }, null, 2));
  assert.equal(report.carrier.error, undefined, "native carrier probe failed before qualification");
  return report;
}

function assertWorkbenchIntact({ baseline, effects }) {
  assert.equal(baseline.calls, 1, "the trusted binding positive control must reach Go");
  assert.equal(baseline.ready, 1, "the trusted native event positive control must settle once");
  assert.equal(baseline.peerPackets, 0, "the trusted workbench must not issue WebRTC probes");
  assert.equal(
    baseline.resourceRequests,
    0,
    "the trusted host must not issue plugin resource probes",
  );
  assert.equal(
    effects.resourceRequests,
    baseline.resourceRequests,
    "a blocked plugin request reached the native asset handler",
  );
  assert.equal(effects.calls, baseline.calls, "an opaque-origin frame reached a native binding");
  assert.equal(
    effects.ready,
    baseline.ready,
    "an opaque-origin frame advanced native window state",
  );
}
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
  if (mode === "isolated-webkit") {
    const control = await observe(executable, "isolated-webkit-control");
    assertWorkbenchIntact(control);
    assert.equal(control.isolation.lockdownEnabled, false);
    assert.equal(control.carrier.frame.observations.peer.available, true);
    assert.equal(control.carrier.frame.observations.peer.attempted, true);
    assert.ok(
      control.effects.peerPackets > 0,
      "the unprotected native control must reach the UDP receiver",
    );
    assert.throws(
      () =>
        assertCarrierIsolation(control.carrier, control.effects.peerPackets, {
          kind: "webkit-lockdown",
          lockdownEnabled: control.isolation.lockdownEnabled,
        }),
      /a plugin WebRTC packet/,
    );
  }
  const report = await observe(executable, mode);
  const { carrier: result, effects, isolation } = report;
  const enforcement =
    mode === "wails"
      ? { kind: "connection-allowlist" }
      : { kind: "webkit-lockdown", lockdownEnabled: isolation.lockdownEnabled };
  assertCarrierIsolation(result, effects.peerPackets, enforcement);
  if (mode === "isolated-webkit")
    assert.throws(
      () =>
        assertCarrierIsolation(result, effects.peerPackets, {
          kind: "webkit-lockdown",
          lockdownEnabled: false,
        }),
      /native WebKit must own the security configuration/,
    );
  assertWorkbenchIntact(report);
  assert.equal(result.wails, mode === "wails");
  if (mode === "wails") {
    assert.equal(result.hostOrigin, "wails://localhost");
  } else {
    assert.equal(new URL(result.hostOrigin).hostname, "127.0.0.1");
    assert.equal(
      result.frame.observations.nativeHandler,
      false,
      "the isolated carrier must have no Wails message handler",
    );
    assert.equal(
      result.frame.observations.wailsGlobal,
      false,
      "the isolated carrier must have no Wails runtime injection",
    );
    assert.equal(
      result.frame.observations.carrierHandler,
      true,
      "the child must attempt the native publication boundary",
    );
    assert.ok(
      isolation.rejectedMessages > 0,
      "native frame identity must reject guest control attempts",
    );
    assert.equal(
      effects.childMessages,
      0,
      "the isolated guest must not reach the workbench native handler",
    );
  }
} finally {
  await rm(directory, { recursive: true, force: true });
}
