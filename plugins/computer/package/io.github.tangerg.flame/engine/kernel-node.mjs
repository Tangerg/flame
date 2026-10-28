import {
  closeSync,
  constants,
  lstatSync,
  openSync,
  realpathSync,
  unlinkSync,
} from "node:fs";
import { randomUUID } from "node:crypto";
import { basename, dirname, isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";
import { configuration, trustedServices } from "./sandbox-config.mjs";

export class KernelRefusal extends Error {
  constructor() {
    super("computer: unconfined or untrusted native Node launch");
    this.name = "KernelRefusal";
  }
}

function refuse() {
  throw new KernelRefusal();
}

function physical(path, kind, privateFile = false) {
  if (!isAbsolute(path) || realpathSync(path) !== path) refuse();
  const info = lstatSync(path);
  if (!(kind === "directory" ? info.isDirectory() : info.isFile())) refuse();
  if (
    privateFile &&
    (info.uid !== process.geteuid() || (info.mode & 0o022) !== 0)
  )
    refuse();
}

function denied(cause) {
  return ["ENOENT", "EACCES", "EPERM", "EROFS"].includes(cause?.code);
}

function confined(config) {
  // A failed native availability probe may make node_repl choose direct launch.
  // The host creates and retains this private boundary until the engine is joined.
  let readable;
  try {
    readable = openSync(
      config.sentinel,
      constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK,
    );
  } catch (cause) {
    if (!denied(cause)) throw cause;
  }
  if (readable !== undefined) {
    closeSync(readable);
    refuse();
  }
  const destination = join(
    dirname(config.sentinel),
    `kernel-write-${randomUUID()}`,
  );
  let writable;
  try {
    writable = openSync(
      destination,
      constants.O_WRONLY |
        constants.O_CREAT |
        constants.O_EXCL |
        constants.O_NOFOLLOW |
        constants.O_NONBLOCK,
      0o600,
    );
  } catch (cause) {
    if (!denied(cause)) throw cause;
  }
  if (writable !== undefined) {
    try {
      closeSync(writable);
    } finally {
      unlinkSync(destination);
    }
    refuse();
  }
}

export function decide(args, environment) {
  try {
    if (
      !Array.isArray(args) ||
      args.length > 6 ||
      args.some(
        (part) =>
          typeof part !== "string" || /[\p{Cc}\p{Surrogate}]/u.test(part),
      )
    )
      refuse();
    const config = configuration(environment);
    physical(config.node, "file");
    physical(config.runtime, "directory");
    physical(config.workdir, "directory", true);
    if (args.length === 1 && args[0] === "--version")
      return { command: config.node, args: [...args] };
    if (args[0] !== "--experimental-vm-modules") refuse();
    const script = args[1];
    if (typeof script !== "string" || !isAbsolute(script)) refuse();
    const directory = dirname(script);
    if (
      dirname(directory) !== config.tempDirectory ||
      !/^\.tmp[A-Za-z0-9]+$/.test(basename(directory))
    )
      refuse();
    physical(directory, "directory", true);
    physical(script, "file", true);
    physical(join(directory, "kernel.js"), "file", true);
    if (
      args.length === 6 &&
      basename(script) === "kernel.js" &&
      args[2] === "--session-id" &&
      args[3].length > 0 &&
      args[3].length <= 256 &&
      args[4] === "--working-dir" &&
      args[5] === config.workdir
    ) {
      confined(config);
      return { command: config.node, args: [...args] };
    }
    if (
      args.length === 3 &&
      basename(script) === "trusted-worker.js" &&
      args[2] === config.workdir
    ) {
      for (const path of trustedServices(environment, config).files)
        physical(path, "file");
      // A trusted worker retains its caller's confinement. Only the separate
      // Linux sandbox launcher can select its authenticated host execution path.
      return { command: config.node, args: [...args] };
    }
    refuse();
  } catch (cause) {
    if (cause instanceof KernelRefusal) throw cause;
    throw new KernelRefusal();
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    if (process.platform !== "linux" && process.platform !== "darwin") refuse();
    const selected = decide(process.argv.slice(2), process.env);
    process.execve(selected.command, [selected.command, ...selected.args]);
  } catch {
    process.stderr.write("computer: native sandbox Node launch refused\n");
    process.exitCode = 70;
  }
}
