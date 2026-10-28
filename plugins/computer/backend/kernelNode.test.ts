import { execFile } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  realpathSync,
  rmSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, expect, it } from "vitest";
import {
  decide,
  KernelRefusal,
} from "../package/io.github.tangerg.flame/engine/kernel-node.mjs";

const roots: string[] = [];
afterEach(() => {
  for (const root of roots.splice(0))
    rmSync(root, { recursive: true, force: true });
});

function file(path: string, source = "", mode = 0o600): string {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  writeFileSync(path, source, { mode });
  return path;
}

function fixture() {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "flame-kernel-node-")));
  roots.push(root);
  const runtime = join(root, "resources/cua_node");
  const node = file(
    join(runtime, "bin/node"),
    '#!/bin/sh\nprintf "executed" > "$FLAME_TEST_EXECUTION"\n',
    0o700,
  );
  const codex = file(join(root, "resources/codex"), "", 0o700);
  const workdir = join(root, "lease/work");
  const tempDirectory = join(root, "lease/control/tmp");
  const extraction = join(tempDirectory, ".tmpOriginal12");
  const sentinel = file(
    join(root, "lease/control/private/kernel-boundary"),
    "host control boundary",
  );
  const launcher = file(join(root, "lease/control/launch/bin/node"));
  const guardDirectory = realpathSync(
    fileURLToPath(
      new URL("../package/io.github.tangerg.flame/engine", import.meta.url),
    ),
  );
  for (const path of [workdir, extraction])
    mkdirSync(path, { recursive: true, mode: 0o700 });
  const kernel = file(join(extraction, "kernel.js"));
  const worker = file(join(extraction, "trusted-worker.js"));
  for (const name of ["sky", "browser-desktop"])
    file(join(runtime, "lib/node_modules/@oai", name, "package.json"), "{}");
  const configuration = {
    runtime,
    node,
    codex,
    workdir,
    tempDirectory,
    guardDirectory,
    launcher,
    sentinel,
    filesystem: {
      ":minimal": "read",
      [dirname(runtime)]: "read",
      [guardDirectory]: "read",
      [dirname(launcher)]: "read",
      [workdir]: "write",
    },
  };
  return {
    root,
    configuration,
    kernel: [
      "--experimental-vm-modules",
      kernel,
      "--session-id",
      "owned-turn",
      "--working-dir",
      workdir,
    ],
    worker: ["--experimental-vm-modules", worker, workdir],
    environment: {
      FLAME_COMPUTER_SANDBOX: JSON.stringify(configuration),
      NODE_REPL_TRUSTED_SERVICES: JSON.stringify({ sky: "@oai/sky/service" }),
    },
  };
}

it("refuses the original engine's direct kernel fallback before loading any JavaScript", async () => {
  const f = fixture();
  const execution = join(f.root, "executed");
  await expect(
    promisify(execFile)(
      process.execPath,
      [join(f.configuration.guardDirectory, "kernel-node.mjs"), ...f.kernel],
      {
        env: { ...f.environment, FLAME_TEST_EXECUTION: execution },
        timeout: 5000,
      },
    ),
  ).rejects.toMatchObject({ code: 70 });
  expect(existsSync(execution)).toBe(false);
  expect(readdirSync(dirname(f.configuration.sentinel))).toEqual([
    "kernel-boundary",
  ]);
});

it("also refuses when a missing sentinel leaves the private control directory writable", () => {
  const f = fixture();
  unlinkSync(f.configuration.sentinel);
  expect(() => decide(f.kernel, f.environment)).toThrow(KernelRefusal);
  expect(readdirSync(dirname(f.configuration.sentinel))).toEqual([]);
});

it("preserves the original trusted worker's inherited confinement and exact arguments", () => {
  const f = fixture();
  for (const sky of [
    "@oai/sky/service",
    join(f.configuration.guardDirectory, "native-sky.mjs"),
  ]) {
    const environment = {
      ...f.environment,
      NODE_REPL_TRUSTED_SERVICES: JSON.stringify({
        sky,
        browser: "@oai/browser-desktop/service",
      }),
    };
    expect(decide(f.worker, environment)).toEqual({
      command: f.configuration.node,
      args: f.worker,
    });
  }
});

it("answers the original engine's exact version probe with the selected Node", () => {
  const f = fixture();
  expect(decide(["--version"], f.environment)).toEqual({
    command: f.configuration.node,
    args: ["--version"],
  });
});

it("refuses alternate Node entrypoints, unknown switches, services and extracted code", () => {
  const f = fixture();
  for (const args of [
    ["-v"],
    ["--version", "--eval", "untrusted"],
    ["--eval", "untrusted"],
    [...f.worker, "--inspect"],
    ["--experimental-vm-modules", f.worker[1]!, f.root],
    [
      "--experimental-vm-modules",
      file(join(f.configuration.workdir, "trusted-worker.js")),
      f.configuration.workdir,
    ],
  ])
    expect(() => decide(args, f.environment)).toThrow(KernelRefusal);
  expect(() =>
    decide(f.worker, {
      ...f.environment,
      NODE_REPL_TRUSTED_SERVICES: JSON.stringify({ sky: "/tmp/other.mjs" }),
    }),
  ).toThrow(KernelRefusal);
  const worker = f.worker[1]!;
  chmodSync(worker, 0o666);
  expect(() => decide(f.worker, f.environment)).toThrow(KernelRefusal);
  unlinkSync(worker);
  symlinkSync(file(join(f.root, "other.js")), worker);
  expect(() => decide(f.worker, f.environment)).toThrow(KernelRefusal);
});

it("cannot move its confinement sentinel into an allowed read or write root", () => {
  const f = fixture();
  for (const sentinel of [
    join(f.configuration.workdir, "sentinel"),
    join(dirname(f.configuration.runtime), "sentinel"),
    join(f.configuration.guardDirectory, "sentinel"),
    join(dirname(f.configuration.launcher), "sentinel"),
  ])
    expect(() =>
      decide(f.worker, {
        ...f.environment,
        FLAME_COMPUTER_SANDBOX: JSON.stringify({
          ...f.configuration,
          sentinel,
        }),
      }),
    ).toThrow(KernelRefusal);
});
