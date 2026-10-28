import { lstat, mkdir, realpath, writeFile } from "node:fs/promises";
import { dirname, isAbsolute, join, relative } from "node:path";
import { sandboxFilesystem, sandboxState } from "./sandbox";
import { verifyConfinement } from "./confinement";
import {
  configuration as sandboxConfiguration,
  CONFIGURATION,
} from "../package/io.github.tangerg.flame/engine/sandbox-config.mjs";

export interface SandboxOptions {
  readonly runtime: string;
  readonly node: string;
  readonly codex: string;
  readonly packageRoot: string;
  readonly workdir: string;
  readonly controlDirectory: string;
  readonly environment: Readonly<Record<string, string>>;
}

function within(path: string, directory: string): boolean {
  const difference = relative(directory, path);
  return (
    difference === "" ||
    (difference !== ".." &&
      !difference.startsWith("../") &&
      !isAbsolute(difference))
  );
}

async function privateDirectory(path: string): Promise<void> {
  if (!isAbsolute(path) || (await realpath(path)) !== path)
    throw new Error("computer: sandbox directory must be canonical");
  const info = await lstat(path);
  if (
    !info.isDirectory() ||
    info.uid !== process.geteuid!() ||
    (info.mode & 0o022) !== 0
  )
    throw new Error("computer: sandbox directory must be private");
}

export async function prepareSandbox(
  options: SandboxOptions,
  signal: AbortSignal,
): Promise<{
  environment: Record<string, string>;
  sandbox: ReturnType<typeof sandboxState>;
}> {
  signal.throwIfAborted();
  if (process.platform !== "linux" && process.platform !== "darwin")
    throw new Error("computer: native sandbox requires a supported host");
  await privateDirectory(options.controlDirectory);
  await privateDirectory(options.workdir);
  if (within(options.controlDirectory, options.workdir))
    throw new Error("computer: control files cannot be writable by the kernel");
  for (const path of [
    options.runtime,
    options.node,
    options.codex,
    options.packageRoot,
  ]) {
    if (!isAbsolute(path) || (await realpath(path)) !== path)
      throw new Error(
        "computer: sandbox executable selection is not canonical",
      );
  }
  if (
    !within(options.node, options.runtime) ||
    [options.runtime, options.codex, options.packageRoot].some((path) =>
      within(path, options.workdir),
    )
  )
    throw new Error("computer: sandbox Node is outside the selected runtime");
  const tempDirectory = join(options.controlDirectory, "tmp");
  try {
    await mkdir(tempDirectory, { mode: 0o700 });
  } catch (cause) {
    if ((cause as NodeJS.ErrnoException).code !== "EEXIST") throw cause;
    await privateDirectory(tempDirectory);
  }
  const guardDirectory = join(
    options.packageRoot,
    "io.github.tangerg.flame/engine",
  );
  const launcherDirectory = join(options.controlDirectory, "launch/bin");
  await mkdir(launcherDirectory, { recursive: true, mode: 0o700 });
  await privateDirectory(launcherDirectory);
  const privatePath = join(options.controlDirectory, "private");
  await mkdir(privatePath, { mode: 0o700 });
  const sentinel = join(privatePath, "kernel-boundary");
  await writeFile(sentinel, "owned kernel confinement boundary", {
    mode: 0o600,
    flag: "wx",
  });
  const launcher = join(launcherDirectory, "node");
  const sandbox = sandboxState({
    workdir: options.workdir,
    resources: dirname(options.runtime),
    guardDirectory,
    launcherDirectory,
  });
  const filesystem = sandboxFilesystem(sandbox);
  await verifyConfinement(
    {
      codex: options.codex,
      sandbox,
      workdir: options.workdir,
      sentinel,
      environment: { ...options.environment, TMPDIR: tempDirectory },
    },
    signal,
  );
  signal.throwIfAborted();
  const quote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const writeLauncher = async (path: string, name: string): Promise<void> => {
    const script = join(guardDirectory, name);
    if ((await realpath(script)) !== script || !(await lstat(script)).isFile())
      throw new Error(
        "computer: native sandbox launcher resource is unavailable",
      );
    await writeFile(
      path,
      `#!/bin/sh\nexec ${quote(options.node)} ${quote(script)} "$@"\n`,
      { mode: 0o700, flag: "wx" },
    );
  };
  await writeLauncher(launcher, "kernel-node.mjs");
  const sandboxLauncher = join(options.controlDirectory, "sandbox");
  if (process.platform === "linux")
    await writeLauncher(sandboxLauncher, "linux-sandbox.mjs");
  const allowlist = new Set(
    (options.environment.NODE_REPL_UNTRUSTED_ENV_ALLOWLIST ?? "")
      .split(",")
      .filter(Boolean),
  );
  allowlist.add(CONFIGURATION);
  const configuration = JSON.stringify({
    runtime: options.runtime,
    node: options.node,
    codex: options.codex,
    workdir: options.workdir,
    tempDirectory,
    guardDirectory,
    launcher,
    sentinel,
    filesystem,
  });
  sandboxConfiguration({ [CONFIGURATION]: configuration });
  return {
    sandbox,
    environment: {
      TMPDIR: tempDirectory,
      NODE_REPL_NODE_PATH: launcher,
      CODEX_CLI_PATH:
        process.platform === "linux" ? sandboxLauncher : options.codex,
      NODE_REPL_UNTRUSTED_ENV_ALLOWLIST: [...allowlist].join(","),
      [CONFIGURATION]: configuration,
    },
  };
}
