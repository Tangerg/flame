import { dirname, join } from "node:path";

export interface PrivateDirectory {
  root: string;
  input: string;
  output: string;
  home: string;
  temporary: string;
}

export interface ProcessPlan {
  command: string;
  args: string[];
  cwd: string;
  env: Record<string, string>;
  containment:
    { kind: "namespace" } | { kind: "single"; residentLimitBytes: number };
}

export const DARWIN_SHELL = "/bin/bash";

export interface ProcessBounds {
  memoryBytes: number;
  cpuSeconds: number;
  maximumFileBytes: number;
  openFiles?: number;
  writable: boolean;
}

export const SYSTEM_ROOTS = {
  linux: ["/usr", "/bin", "/lib", "/lib64"],
  darwin: ["/usr", "/bin", "/System", "/Library/Fonts"],
} as const;

export const SYSTEM_FILES = [
  "/etc/fonts",
  "/etc/ld.so.cache",
  "/etc/localtime",
  "/var/cache/fontconfig",
] as const;

export function confinedPlan(
  platform: "linux" | "darwin",
  directory: PrivateDirectory,
  roots: readonly string[],
  executable: string,
  args: readonly string[],
  bounds: ProcessBounds,
  environment: Readonly<Record<string, string>> = {},
): ProcessPlan {
  const { writable, ...resources } = bounds;
  if (typeof writable !== "boolean")
    throw new TypeError("deliverables: process write authority is required");
  for (const [name, value] of Object.entries(resources))
    if (
      !Number.isSafeInteger(value) ||
      value < (name === "maximumFileBytes" ? 0 : 1)
    )
      throw new RangeError(`deliverables: invalid process bound ${name}`);
  const env = {
    ...environment,
    HOME: directory.home,
    TMPDIR: directory.temporary,
    XDG_CACHE_HOME: join(directory.home, "cache"),
    XDG_CONFIG_HOME: join(directory.home, "config"),
    XDG_DATA_HOME: join(directory.home, "data"),
    PATH: "/usr/bin:/bin",
    LANG: "C.UTF-8",
    LC_ALL: "C.UTF-8",
    TZ: "UTC",
  };
  if (platform === "linux") {
    const commandArgs = [
      "--unshare-all",
      "--die-with-parent",
      "--new-session",
      "--cap-drop",
      "ALL",
      // Confined tools have no reason to inspect process metadata. An empty proc
      // directory also avoids requiring a procfs mount in nested sandboxes.
      "--dir",
      "/proc",
      "--dev",
      "/dev",
      "--tmpfs",
      "/tmp",
      "--dir",
      "/etc",
    ];
    for (const root of [...SYSTEM_ROOTS.linux, ...SYSTEM_FILES])
      commandArgs.push("--ro-bind-try", root, root);
    for (const root of roots) commandArgs.push("--ro-bind", root, root);
    commandArgs.push(
      writable ? "--bind" : "--ro-bind",
      directory.root,
      directory.root,
      "--ro-bind",
      directory.input,
      directory.input,
      "--chdir",
      directory.root,
      "--clearenv",
    );
    if (!writable)
      for (const root of ["/", "/tmp", "/dev"])
        commandArgs.push("--remount-ro", root);
    for (const [name, value] of Object.entries(env))
      commandArgs.push("--setenv", name, value);
    commandArgs.push(
      "--",
      "/usr/bin/prlimit",
      `--as=${bounds.memoryBytes}`,
      `--fsize=${bounds.maximumFileBytes}`,
      `--cpu=${bounds.cpuSeconds}`,
      ...(bounds.openFiles === undefined
        ? []
        : [`--nofile=${bounds.openFiles}`]),
      "--core=0",
      "--",
      executable,
      ...args,
    );
    return {
      command: "/usr/bin/bwrap",
      args: commandArgs,
      cwd: directory.root,
      env,
      containment: { kind: "namespace" },
    };
  }
  const quote = (value: string) => JSON.stringify(value);
  const readRoots = [
    ...SYSTEM_ROOTS.darwin,
    ...SYSTEM_FILES,
    ...roots,
    directory.root,
  ];
  const ancestors = new Set<string>();
  for (const root of readRoots)
    for (
      let parent = dirname(root);
      !ancestors.has(parent);
      parent = dirname(parent)
    ) {
      ancestors.add(parent);
      if (parent === "/") break;
    }
  const profile = [
    "(version 1)",
    "(deny default)",
    "(deny network*)",
    // A process group cannot contain a daemon that calls setsid(). Actual Mac
    // binaries run without fork; launcher scripts requiring helpers are refused.
    "(deny process-fork)",
    `(allow process-exec (literal ${quote(DARWIN_SHELL)}) (literal ${quote(executable)}))`,
    "(allow signal (target self))",
    "(allow sysctl-read)",
    // dyld reads the root directory itself during startup, not its descendants.
    '(allow file-read-data (literal "/"))',
    // Canonical path resolution needs ancestor metadata, without sibling data.
    `(allow file-read-metadata ${[...ancestors].map((root) => `(literal ${quote(root)})`).join(" ")})`,
    `(allow file-read* ${readRoots.map((root) => `(subpath ${quote(root)})`).join(" ")} (literal "/dev/null") (literal "/dev/urandom"))`,
    `(allow file-write* ${
      writable
        ? [directory.output, directory.home, directory.temporary]
            .map((root) => `(subpath ${quote(root)})`)
            .join(" ")
        : ""
    } (literal "/dev/null"))`,
  ].join("\n");
  // Only fixed shell builtins are used. All tool paths/arguments remain separate
  // positional arguments, including the user-profile URI.
  // Darwin's shared mappings exceed the working-memory budget before execution.
  // The process owner measures RSS instead of applying an unusable RLIMIT_AS.
  const limits = `ulimit -f ${Math.floor(bounds.maximumFileBytes / 1024)}; ulimit -t ${bounds.cpuSeconds}; ${bounds.openFiles === undefined ? "" : `ulimit -n ${bounds.openFiles}; `}ulimit -c 0; exec "$@"`;
  return {
    command: "/usr/bin/sandbox-exec",
    args: [
      "-p",
      profile,
      DARWIN_SHELL,
      "-eu",
      "-c",
      limits,
      "flame-deliverables",
      executable,
      ...args,
    ],
    cwd: directory.root,
    env,
    containment: { kind: "single", residentLimitBytes: bounds.memoryBytes },
  };
}
