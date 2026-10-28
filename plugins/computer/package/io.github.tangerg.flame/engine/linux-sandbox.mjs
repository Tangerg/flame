import { lstatSync, realpathSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { configuration, trustedServices } from "./sandbox-config.mjs";
const PREFIX = [
  "sandbox",
  "-c",
  'shell_environment_policy.inherit="all"',
  "-c",
  'default_permissions="node_repl"',
  "-c",
];
const PROBE =
  'test -r /etc/os-release || exit 10; touch "$1" && exit 11; exit 12';

export class SandboxRefusal extends Error {
  constructor() {
    super("computer: unsupported or untrusted native sandbox launch");
    this.name = "SandboxRefusal";
  }
}

function refuse() {
  throw new SandboxRefusal();
}

function within(path, directory) {
  const difference = relative(directory, path);
  return (
    difference === "" ||
    (difference !== ".." &&
      !difference.startsWith("../") &&
      !isAbsolute(difference))
  );
}

function physical(path, kind) {
  if (
    typeof path !== "string" ||
    !isAbsolute(path) ||
    /[\p{Cc}\p{Surrogate}]/u.test(path) ||
    realpathSync(path) !== path
  )
    refuse();
  const info = lstatSync(path);
  if (!(kind === "directory" ? info.isDirectory() : info.isFile())) refuse();
  return info;
}

function privatePath(path, kind) {
  const info = physical(path, kind);
  if (info.uid !== process.geteuid() || (info.mode & 0o022) !== 0) refuse();
  return info;
}

// The selected engine emits this bounded inline-table vocabulary. Reject unknown
// syntax instead of interpreting a broader TOML language at an execution boundary.
function profile(source) {
  if (typeof source !== "string" || Buffer.byteLength(source) > 65536) refuse();
  const match =
    /^permissions\.node_repl=\{[ \t]*filesystem[ \t]*=[ \t]*\{(.*)\}[ \t]*,[ \t]*network[ \t]*=[ \t]*\{[ \t]*enabled[ \t]*=[ \t]*false[ \t]*\}[ \t]*\}$/.exec(
      source,
    );
  if (!match) refuse();
  const body = match[1];
  const entry =
    /[ \t]*("(?:[^"\\]|\\(?:["\\bfnrt]|u[0-9a-fA-F]{4}))*")[ \t]*=[ \t]*"(read|write)"[ \t]*(,|$)/y;
  const result = new Map();
  let at = 0;
  while (at < body.length) {
    entry.lastIndex = at;
    const field = entry.exec(body);
    if (!field) refuse();
    const path = JSON.parse(field[1]);
    if (
      typeof path !== "string" ||
      /[\p{Cc}\p{Surrogate}]/u.test(path) ||
      result.has(path) ||
      result.size >= 64
    )
      refuse();
    result.set(path, field[2]);
    at = entry.lastIndex;
    if (field[3] === "," && at === body.length) refuse();
  }
  if (!result.size) refuse();
  return result;
}

function extractedScript(script, config) {
  if (typeof script !== "string" || !isAbsolute(script)) refuse();
  const directory = dirname(script);
  if (
    dirname(directory) !== config.tempDirectory ||
    !/^\.tmp[A-Za-z0-9]+$/.test(basename(directory))
  )
    refuse();
  privatePath(directory, "directory");
  privatePath(script, "file");
  privatePath(join(directory, "kernel.js"), "file");
  return directory;
}

export function decide(args, environment, parentExecutable) {
  try {
    if (
      !Array.isArray(args) ||
      args.length > 32 ||
      args.some((part) => typeof part !== "string" || part.includes("\0")) ||
      PREFIX.some((part, index) => args[index] !== part) ||
      args[7] !== "--"
    )
      refuse();
    const config = configuration(environment);
    physical(config.runtime, "directory");
    physical(config.node, "file");
    physical(config.codex, "file");
    physical(config.guardDirectory, "directory");
    privatePath(config.workdir, "directory");
    privatePath(config.tempDirectory, "directory");
    privatePath(config.launcher, "file");
    if (environment.TMPDIR !== config.tempDirectory) refuse();

    const nodeRepl = realpathSync(join(config.runtime, "bin/node_repl"));
    if (parentExecutable !== nodeRepl || !within(nodeRepl, config.runtime))
      refuse();
    const permission = profile(args[6]);
    const command = args.slice(8);
    if (
      command.length === 5 &&
      command[0] === "/bin/sh" &&
      command[1] === "-c" &&
      command[2] === PROBE &&
      command[3] === "node-repl-sandbox-probe" &&
      basename(command[4]) === "write-must-fail" &&
      dirname(dirname(command[4])) === config.tempDirectory &&
      /^\.tmp[A-Za-z0-9]+$/.test(basename(dirname(command[4]))) &&
      permission.size === 1 &&
      permission.get(":root") === "read"
    )
      return { command: config.codex, args: [...args] };
    if (
      command[0] !== config.launcher ||
      command[1] !== "--experimental-vm-modules"
    )
      refuse();
    const directory = extractedScript(command[2], config);
    for (const [path, access] of Object.entries(config.filesystem)) {
      if (permission.get(path) !== access) refuse();
    }
    for (const [path, access] of permission) {
      if (Object.hasOwn(config.filesystem, path)) continue;
      if (
        access !== "read" ||
        !isAbsolute(path) ||
        (path !== directory && !within(path, dirname(config.runtime)))
      )
        refuse();
      physical(path, "directory");
    }
    if (
      command.length === 7 &&
      basename(command[2]) === "kernel.js" &&
      command[3] === "--session-id" &&
      command[4].length > 0 &&
      command[4].length <= 256 &&
      command[5] === "--working-dir" &&
      command[6] === config.workdir
    )
      return { command: config.codex, args: [...args] };
    if (
      command.length === 4 &&
      basename(command[2]) === "trusted-worker.js" &&
      command[3] === config.workdir
    ) {
      const services = trustedServices(environment, config);
      if (services.sky !== "@oai/sky/service") refuse();
      for (const path of services.files) physical(path, "file");
      return { command: config.node, args: command.slice(1) };
    }
    refuse();
  } catch (cause) {
    if (cause instanceof SandboxRefusal) throw cause;
    throw new SandboxRefusal();
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    if (process.platform !== "linux") refuse();
    const choice = decide(
      process.argv.slice(2),
      process.env,
      realpathSync(`/proc/${process.ppid}/exe`),
    );
    process.execve(choice.command, [choice.command, ...choice.args]);
  } catch {
    process.stderr.write("computer: native sandbox launch refused\n");
    process.exitCode = 70;
  }
}
