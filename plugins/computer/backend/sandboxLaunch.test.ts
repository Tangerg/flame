import { execFile } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
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
import { afterEach, describe, expect, it } from "vitest";
import {
  decide,
  SandboxRefusal,
} from "../package/io.github.tangerg.flame/engine/linux-sandbox.mjs";
import { prepareSandbox, type SandboxOptions } from "./sandboxLaunch";
import { sandboxState } from "./sandbox";

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

describe("native CUA sandbox launch", () => {
  it("keeps generated JavaScript under the original sandbox with the exact admitted arguments", () => {
    const f = fixture();
    const args = f.args(f.kernel);
    expect(decide(args, f.env, f.parent)).toEqual({
      command: f.options.codex,
      args,
    });
  });

  it("allows only the selected engine's genuine trusted desktop worker to leave the kernel sandbox", () => {
    const f = fixture();
    for (const services of [
      { sky: "@oai/sky/service" },
      { sky: "@oai/sky/service", browser: "@oai/browser-desktop/service" },
    ]) {
      expect(
        decide(
          f.args(f.worker),
          { ...f.env, NODE_REPL_TRUSTED_SERVICES: JSON.stringify(services) },
          f.parent,
        ),
      ).toEqual({
        command: f.options.node,
        args: f.worker.slice(1),
      });
    }
  });

  it("passes only the engine's fixed availability probe to the real sandbox", () => {
    const f = fixture();
    const command = [
      "/bin/sh",
      "-c",
      'test -r /etc/os-release || exit 10; touch "$1" && exit 11; exit 12',
      "node-repl-sandbox-probe",
      join(f.extraction, "write-must-fail"),
    ];
    const args = f.args(command, { ":root": "read" });
    expect(decide(args, f.env, f.parent)).toEqual({
      command: f.options.codex,
      args,
    });
    command[2] = "exit 12";
    expect(() =>
      decide(f.args(command, { ":root": "read" }), f.env, f.parent),
    ).toThrow(SandboxRefusal);
  });

  it("refuses network access, broader reads, other writes, duplicate rules and unknown profile syntax", () => {
    const f = fixture();
    const sources = [
      f.args(f.kernel)[6]!.replace("enabled = false", "enabled = true"),
      f.args(f.kernel, { ...f.rules, ":root": "read" })[6]!,
      f.args(f.kernel, {
        ...f.rules,
        [f.options.controlDirectory]: "write",
      })[6]!,
      f.args(f.kernel, { ...f.rules, [f.root]: "read" })[6]!,
      f.args(f.kernel, { ...f.rules, ":minimal": "write" })[6]!,
      f
        .args(f.kernel)[6]!
        .replace(
          '":minimal" = "read"',
          '":minimal" = "read", ":minimal" = "write"',
        ),
      f.args(f.kernel)[6]!.replace("network =", "other = {}, network ="),
      f
        .args(f.kernel)[6]!
        .replace("filesystem = {", "filesystem = {evil = true, "),
      f.args(f.kernel)[6]!.replace('":minimal"', "':minimal'"),
    ];
    for (const source of sources) {
      const args = f.args(f.kernel);
      args[6] = source;
      expect(() => decide(args, f.env, f.parent)).toThrow(SandboxRefusal);
    }
  });

  it("rejects altered binaries, parents, arguments and unrecognized commands", () => {
    const f = fixture();
    const commands = [
      [process.execPath, ...f.kernel.slice(1)],
      [f.options.node, "--inspect", ...f.kernel.slice(2)],
      [...f.worker, "--eval", "malicious"],
      [
        f.options.node,
        "--experimental-vm-modules",
        join(f.extraction, "other.js"),
        f.options.workdir,
      ],
      [...f.kernel.slice(0, -1), f.root],
      ["/bin/sh", "-c", "true"],
    ];
    for (const command of commands)
      expect(() => decide(f.args(command), f.env, f.parent)).toThrow(
        SandboxRefusal,
      );
    expect(() => decide(f.args(f.worker), f.env, process.execPath)).toThrow(
      SandboxRefusal,
    );
    expect(() => decide(["--version"], f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
    expect(() =>
      decide(
        [
          ...f.args(f.kernel).slice(0, 7),
          "--full-auto",
          ...f.args(f.kernel).slice(7),
        ],
        f.env,
        f.parent,
      ),
    ).toThrow(SandboxRefusal);
  });

  it("rejects writable, linked, or misplaced extracted worker code", () => {
    const f = fixture();
    const script = f.worker[2]!;
    chmodSync(script, 0o666);
    expect(() => decide(f.args(f.worker), f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
    chmodSync(script, 0o600);
    chmodSync(f.extraction, 0o770);
    expect(() => decide(f.args(f.worker), f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
    chmodSync(f.extraction, 0o700);
    unlinkSync(script);
    symlinkSync(file(join(f.options.workdir, "trusted-worker.js")), script);
    expect(() => decide(f.args(f.worker), f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
    unlinkSync(script);
    file(script);
    unlinkSync(f.kernel[2]!);
    expect(() => decide(f.args(f.worker), f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
  });

  it("rejects custom service modules, missing selected packages and untrusted configuration", () => {
    const f = fixture();
    for (const services of [
      {},
      { sky: "@oai/sky/service", extra: "@oai/sky/service" },
      { sky: "/tmp/service.mjs" },
      { sky: "@oai/sky/service", browser: "/tmp/browser.mjs" },
      [],
    ])
      expect(() =>
        decide(
          f.args(f.worker),
          { ...f.env, NODE_REPL_TRUSTED_SERVICES: JSON.stringify(services) },
          f.parent,
        ),
      ).toThrow(SandboxRefusal);
    for (const value of ["null", "[]", "{}", "malformed"])
      expect(() =>
        decide(
          f.args(f.worker),
          { ...f.env, FLAME_COMPUTER_SANDBOX: value },
          f.parent,
        ),
      ).toThrow(SandboxRefusal);
    unlinkSync(
      join(f.options.runtime, "lib/node_modules/@oai/sky/package.json"),
    );
    expect(() => decide(f.args(f.worker), f.env, f.parent)).toThrow(
      SandboxRefusal,
    );
  });
});

describe.skipIf(process.platform !== "linux")(
  "Linux confinement admission",
  () => {
    it("admits a private launcher only after the native confinement probe succeeds", async () => {
      const f = fixture(false);
      const value = await prepareSandbox(
        f.options,
        new AbortController().signal,
      );
      expect(value.sandbox).toEqual(
        sandboxState({
          workdir: f.options.workdir,
          resources: dirname(f.options.runtime),
          guardDirectory: join(
            f.options.packageRoot,
            "io.github.tangerg.flame/engine",
          ),
          launcherDirectory: join(f.options.controlDirectory, "launch/bin"),
        }),
      );
      expect(value.environment.CODEX_CLI_PATH).toBe(
        join(f.options.controlDirectory, "sandbox"),
      );
      expect(value.environment.TMPDIR).toBe(
        join(f.options.controlDirectory, "tmp"),
      );
      const args = readFileSync(join(f.root, "probe.log"), "utf8");
      expect(args).toContain('":minimal" = "read"');
      expect(args).toContain(`${JSON.stringify(f.options.workdir)} = "write"`);
      expect(args).not.toContain('":root" = "read"');
      expect(args).toContain("flame-computer-confinement-probe");
      const config = JSON.parse(
        value.environment.FLAME_COMPUTER_SANDBOX!,
      ) as Record<string, unknown>;
      expect(config.filesystem).toEqual({
        ":minimal": "read",
        [dirname(f.options.runtime)]: "read",
        [join(f.options.packageRoot, "io.github.tangerg.flame/engine")]: "read",
        [join(f.options.controlDirectory, "launch/bin")]: "read",
        [f.options.workdir]: "write",
      });
    });

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

    it("keeps trusted control and extracted engine files outside the writable working directory", async () => {
      const f = fixture(false);
      const controlDirectory = join(f.options.workdir, "control");
      mkdirSync(controlDirectory, { mode: 0o700 });
      await expect(
        prepareSandbox(
          { ...f.options, controlDirectory },
          new AbortController().signal,
        ),
      ).rejects.toThrow("cannot be writable");
      expect(existsSync(join(f.root, "probe.log"))).toBe(false);
      chmodSync(f.options.controlDirectory, 0o777);
      await expect(
        prepareSandbox(f.options, new AbortController().signal),
      ).rejects.toThrow("must be private");
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

    it("cancels and joins a probe before reporting aborted startup", async () => {
      const f = fixture(false);
      const child = file(
        join(f.root, "probe-child.cjs"),
        "require('node:fs').writeFileSync(process.env.FLAME_TEST_PROBE_LOG, String(process.pid)); setInterval(() => {}, 1000);",
      );
      const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
      writeFileSync(
        f.options.codex,
        `#!/bin/sh\nexec ${quote(process.execPath)} ${quote(child)}\n`,
      );
      const abort = new AbortController();
      const pending = prepareSandbox(f.options, abort.signal);
      const rejection = expect(pending).rejects.toThrow(
        "test startup cancelled",
      );
      const log = join(f.root, "probe.log");
      for (let index = 0; index < 100 && !existsSync(log); index++)
        await new Promise((resolve) => setTimeout(resolve, 10));
      expect(existsSync(log)).toBe(true);
      const pid = Number(readFileSync(log, "utf8"));
      abort.abort(new Error("test startup cancelled"));
      await rejection;
      expect(() => process.kill(pid, 0)).toThrow();
      expect(existsSync(join(f.options.controlDirectory, "sandbox"))).toBe(
        false,
      );
    });
  },
);
