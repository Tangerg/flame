import { execFile } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterEach, describe, expect, it } from "vitest";
import { prepareSandbox, type SandboxOptions } from "./sandboxLaunch";

const roots: string[] = [];
const run = promisify(execFile);
afterEach(() => {
  for (const root of roots.splice(0))
    rmSync(root, { recursive: true, force: true });
});

function file(path: string, source = "", mode = 0o600): string {
  mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
  writeFileSync(path, source, { mode });
  return path;
}

function fixture(launchCreated = true): {
  root: string;
  options: SandboxOptions;
  env: Record<string, string>;
  extraction: string;
  parent: string;
  kernel: string[];
  worker: string[];
  rules: Record<string, string>;
  args(command: string[], rules?: Record<string, string>): string[];
} {
  const root = realpathSync(
    mkdtempSync(join(tmpdir(), "flame-native-sandbox-")),
  );
  roots.push(root);
  const runtime = join(root, "resources/cua_node");
  const node = file(
    join(runtime, "bin/node"),
    "#!/bin/sh\nprintf '%s\\n' \"$@\"\n",
    0o700,
  );
  const parent = file(
    join(runtime, "bin/node_repl"),
    "selected native engine",
    0o700,
  );
  const codex = file(
    join(root, "resources/codex"),
    '#!/bin/sh\nprintf "%s\\n" "$@" > "$FLAME_TEST_PROBE_LOG"\nexit 12\n',
    0o700,
  );
  for (const name of ["sky", "browser-desktop"])
    file(join(runtime, "lib/node_modules/@oai", name, "package.json"), "{}");
  const controlDirectory = join(root, "lease/control");
  const workdir = join(root, "lease/work");
  const tempDirectory = join(controlDirectory, "tmp");
  const extraction = join(tempDirectory, ".tmpTrusted123");
  mkdirSync(workdir, { recursive: true, mode: 0o700 });
  mkdirSync(extraction, { recursive: true, mode: 0o700 });
  file(join(extraction, "kernel.js"));
  file(join(extraction, "trusted-worker.js"));
  const packageRoot = realpathSync(
    fileURLToPath(new URL("../package", import.meta.url)),
  );
  const guardDirectory = join(packageRoot, "io.github.tangerg.flame/engine");
  const launcher = join(controlDirectory, "launch/bin/node");
  mkdirSync(dirname(launcher), { recursive: true, mode: 0o700 });
  if (launchCreated) file(launcher, "selected kernel guard", 0o700);
  const sentinel = join(controlDirectory, "private/kernel-boundary");
  const filesystem = {
    ":minimal": "read",
    [dirname(runtime)]: "read",
    [guardDirectory]: "read",
    [dirname(launcher)]: "read",
    [workdir]: "write",
  };
  const env = {
    TMPDIR: tempDirectory,
    NODE_REPL_TRUSTED_SERVICES: JSON.stringify({ sky: "@oai/sky/service" }),
    FLAME_COMPUTER_SANDBOX: JSON.stringify({
      runtime,
      node,
      codex,
      workdir,
      tempDirectory,
      guardDirectory,
      launcher,
      sentinel,
      filesystem,
    }),
  };
  const rules = {
    ...filesystem,
    [extraction]: "read",
    [join(runtime, "bin")]: "read",
    [join(runtime, "lib/node_modules")]: "read",
  };
  return {
    root,
    options: {
      runtime,
      node,
      codex,
      workdir,
      controlDirectory,
      packageRoot,
      environment: {
        FLAME_TEST_PROBE_LOG: join(root, "probe.log"),
        PATH: "/usr/bin:/bin",
      },
    },
    env,
    parent,
    extraction,
    kernel: [
      launcher,
      "--experimental-vm-modules",
      join(extraction, "kernel.js"),
      "--session-id",
      "owned-call",
      "--working-dir",
      workdir,
    ],
    worker: [
      launcher,
      "--experimental-vm-modules",
      join(extraction, "trusted-worker.js"),
      workdir,
    ],
    rules,
    args: (command, selected = rules) => [
      "sandbox",
      "-c",
      'shell_environment_policy.inherit="all"',
      "-c",
      'default_permissions="node_repl"',
      "-c",
      `permissions.node_repl={filesystem = {${Object.entries(selected)
        .map(
          ([path, access]) =>
            `${JSON.stringify(path)} = ${JSON.stringify(access)}`,
        )
        .join(", ")}}, network = {enabled = false}}`,
      "--",
      ...command,
    ],
  };
}

describe.skipIf(process.platform !== "linux")(
  "Linux confinement admission",
  () => {
    it("refuses failed confinement without installing an unsandboxed alternative", async () => {
      const f = fixture(false);
      writeFileSync(f.options.codex, "#!/bin/sh\nexit 11\n");
      await expect(
        prepareSandbox(f.options, new AbortController().signal),
      ).rejects.toThrow("required kernel confinement is unavailable");
      expect(existsSync(join(f.options.controlDirectory, "sandbox"))).toBe(
        false,
      );
    });

    it("quotes selected paths without evaluating shell syntax", async () => {
      const f = fixture(false);
      const selected = join(f.options.runtime, "bin/node with ' quote");
      file(selected, "#!/bin/sh\nprintf '%s\\n' \"$@\"\n", 0o700);
      const value = await prepareSandbox(
        { ...f.options, node: selected },
        new AbortController().signal,
      );
      const args = [
        "one two",
        "$(printf unexpected)",
        "`printf unexpected`",
        "a'b",
      ];
      const output = await run(value.environment.NODE_REPL_NODE_PATH!, args, {
        env: { PATH: "/usr/bin:/bin" },
      });
      expect(output.stdout.trim().split("\n")).toEqual([
        join(
          f.options.packageRoot,
          "io.github.tangerg.flame/engine/kernel-node.mjs",
        ),
        ...args,
      ]);
    });
  },
);
