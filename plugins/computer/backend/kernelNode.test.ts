import { execFile } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, expect, it } from "vitest";

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
