import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { sandboxFilesystem, sandboxState } from "./sandbox";

interface ConfinementOptions {
  readonly codex: string;
  readonly sandbox: ReturnType<typeof sandboxState>;
  readonly workdir: string;
  readonly sentinel: string;
  readonly environment: Readonly<Record<string, string>>;
}

const PROBE_MS = 30_000;

async function probe(
  command: string,
  args: string[],
  options: { cwd: string; env: Record<string, string> },
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  const lifetime = AbortSignal.any([signal, AbortSignal.timeout(PROBE_MS)]);
  const child = spawn(command, args, {
    ...options,
    shell: false,
    detached: true,
    stdio: "ignore",
  });
  let failure: unknown;
  const abort = () => {
    if (!child.pid) return;
    try {
      process.kill(-child.pid, "SIGKILL");
    } catch (cause) {
      if ((cause as NodeJS.ErrnoException).code !== "ESRCH") failure = cause;
    }
  };
  lifetime.addEventListener("abort", abort, { once: true });
  child.once("spawn", () => {
    if (lifetime.aborted) abort();
  });
  child.once("error", (cause) => {
    failure = cause;
  });
  const status = await new Promise<number | null>((resolve) => {
    child.once("close", resolve);
  });
  lifetime.removeEventListener("abort", abort);
  if (failure)
    throw new Error("computer: sandbox confinement probe failed", {
      cause: failure,
    });
  lifetime.throwIfAborted();
  if (status !== 12)
    throw new Error("computer: required kernel confinement is unavailable");
}

export async function verifyConfinement(
  options: ConfinementOptions,
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  if (process.platform !== "linux" && process.platform !== "darwin")
    throw new Error("computer: native kernel confinement is unsupported");
  const filesystem = sandboxFilesystem(options.sandbox);
  const profile = `permissions.node_repl={filesystem = {${Object.entries(
    filesystem,
  )
    .map(
      ([path, access]) => `${JSON.stringify(path)} = ${JSON.stringify(access)}`,
    )
    .join(", ")}}, network = {enabled = false}}`;
  const suffix = randomUUID();
  const deniedWrite = join(dirname(options.sentinel), `probe-denied-${suffix}`);
  const allowedWrite = join(options.workdir, `probe-allowed-${suffix}`);
  try {
    // Observe the same retained private boundary that the Node guard checks
    // again before every model kernel starts, including native probe fallback.
    await probe(
      options.codex,
      [
        "sandbox",
        "-c",
        'shell_environment_policy.inherit="all"',
        "-c",
        'default_permissions="node_repl"',
        "-c",
        profile,
        "--",
        "/bin/sh",
        "-c",
        'test -r /bin/sh || exit 10; test ! -r "$1" || exit 11; if touch "$2" 2>/dev/null; then exit 13; fi; touch "$3" 2>/dev/null || exit 14; exit 12',
        "flame-computer-confinement-probe",
        options.sentinel,
        deniedWrite,
        allowedWrite,
      ],
      {
        cwd: options.workdir,
        env: {
          ...options.environment,
          CODEX_CLI_PATH: options.codex,
        },
      },
      signal,
    );
  } finally {
    await Promise.all(
      [deniedWrite, allowedWrite].map((path) => rm(path, { force: true })),
    );
  }
}
