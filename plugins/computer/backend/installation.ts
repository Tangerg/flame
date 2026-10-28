import { execFile } from "node:child_process";
import {
  accessSync,
  constants,
  closeSync,
  fstatSync,
  lstatSync,
  openSync,
  readdirSync,
  readSync,
  realpathSync,
  statSync,
} from "node:fs";
import { isAbsolute, join, relative, sep } from "node:path";
import { z } from "zod";

const manifestSchema = z.object({
  platform: z.enum(["linux", "darwin"]),
  arch: z.enum(["x64", "arm64"]),
  runtime_archive_version: z.string().min(1).max(200),
  node_path: z.literal("bin/node"),
  node_repl_path: z.literal("bin/node_repl"),
  node_modules: z.literal("lib/node_modules"),
});
const appSchema = z.object({
  name: z.literal("openai-codex-electron"),
  version: z.string().min(1).max(200),
});

export class InstallationError extends Error {
  constructor(message: string, cause?: unknown) {
    super(`computer: ${message}`, { cause });
    this.name = "InstallationError";
  }
}

function within(path: string, root: string): boolean {
  const child = relative(root, path);
  return (
    child === "" ||
    (!isAbsolute(child) && child !== ".." && !child.startsWith(`..${sep}`))
  );
}

function regular(root: string, path: string, executable = false): string {
  let resolved: string;
  try {
    resolved = realpathSync(path);
    if (!within(resolved, root) || !statSync(resolved).isFile())
      throw new Error("not a contained regular file");
    if (executable) accessSync(resolved, constants.X_OK);
  } catch (cause) {
    throw new InstallationError(
      "selected CUA application is incomplete or contains an external file",
      cause,
    );
  }
  return resolved;
}

function readBounded(path: string, maximum = 1024 * 1024): Buffer {
  const fd = openSync(
    path,
    constants.O_RDONLY | constants.O_NONBLOCK | constants.O_NOFOLLOW,
  );
  try {
    const info = fstatSync(fd);
    if (!info.isFile())
      throw new InstallationError("application metadata is not a regular file");
    const size = info.size;
    if (size > maximum)
      throw new InstallationError("application metadata exceeds its bound");
    const result = readExactly(fd, size, 0);
    if (readSync(fd, Buffer.alloc(1), 0, 1, size))
      throw new InstallationError(
        "application metadata changed while being read",
      );
    return result;
  } finally {
    closeSync(fd);
  }
}

function readExactly(fd: number, length: number, offset: number): Buffer {
  const result = Buffer.alloc(length);
  let done = 0;
  while (done < length) {
    const count = readSync(fd, result, done, length - done, offset + done);
    if (!count) throw new InstallationError("application archive is truncated");
    done += count;
  }
  return result;
}

function appMetadata(archive: string): z.infer<typeof appSchema> {
  const fd = openSync(
    archive,
    constants.O_RDONLY | constants.O_NONBLOCK | constants.O_NOFOLLOW,
  );
  try {
    const info = fstatSync(fd);
    if (!info.isFile())
      throw new InstallationError("application archive is not a regular file");
    const size = info.size;
    const prefix = readExactly(fd, 16, 0);
    const headerSize = prefix.readUInt32LE(4);
    const payloadSize = prefix.readUInt32LE(8);
    const jsonSize = prefix.readUInt32LE(12);
    if (
      prefix.readUInt32LE(0) !== 4 ||
      headerSize < 8 ||
      payloadSize !== headerSize - 4 ||
      jsonSize > payloadSize - 4 ||
      jsonSize > 64 * 1024 * 1024 ||
      8 + headerSize > size
    )
      throw new InstallationError("application archive header is invalid");
    const header = JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(
        readExactly(fd, jsonSize, 16),
      ),
    );
    const entry = header?.files?.["package.json"];
    if (
      !entry ||
      entry.link !== undefined ||
      entry.unpacked ||
      typeof entry.offset !== "string" ||
      !/^\d+$/.test(entry.offset) ||
      !Number.isSafeInteger(entry.size) ||
      entry.size < 0 ||
      entry.size > 1024 * 1024
    )
      throw new InstallationError("application archive metadata is invalid");
    const offset = Number(entry.offset);
    if (
      !Number.isSafeInteger(offset) ||
      offset < 0 ||
      8 + headerSize + offset + entry.size > size
    )
      throw new InstallationError(
        "application archive metadata is out of bounds",
      );
    return appSchema.parse(
      JSON.parse(
        new TextDecoder("utf-8", { fatal: true }).decode(
          readExactly(fd, entry.size, 8 + headerSize + offset),
        ),
      ),
    );
  } finally {
    closeSync(fd);
  }
}

async function system(
  command: string,
  args: string[],
  signal: AbortSignal,
): Promise<string> {
  signal.throwIfAborted();
  return await new Promise((resolve, reject) => {
    let received = false;
    let closed = false;
    let failure: unknown;
    let output = "";
    const settle = () => {
      if (!received || !closed) return;
      if (failure)
        reject(
          new InstallationError(
            "installed application signature or identity could not be verified",
            failure,
          ),
        );
      else resolve(output);
    };
    const child = execFile(
      command,
      args,
      {
        signal,
        timeout: 120_000,
        maxBuffer: 1024 * 1024,
        encoding: "utf8",
        killSignal: "SIGKILL",
        env: { PATH: "/usr/bin:/bin:/usr/sbin:/sbin" },
      },
      (error, stdout, stderr) => {
        received = true;
        failure = error;
        output = stdout + stderr;
        settle();
      },
    );
    child.once("close", () => {
      closed = true;
      settle();
    });
  });
}

async function verifyBundle(
  bundle: string,
  identifier: string,
  signal: AbortSignal,
): Promise<void> {
  const identity = await system(
    "/usr/bin/codesign",
    ["-dv", "--verbose=4", bundle],
    signal,
  );
  const lines = identity.split(/\r?\n/);
  if (
    !lines.includes(`Identifier=${identifier}`) ||
    !lines.includes("TeamIdentifier=2DC432GLL2")
  )
    throw new InstallationError(
      "installed application has an unexpected signing identity",
    );
  await system(
    "/usr/bin/codesign",
    ["--verify", "--deep", "--strict", bundle],
    signal,
  );
}

type Stamp = readonly [bigint, bigint, bigint, bigint, bigint, bigint];

function capture(application: string): Map<string, Stamp> {
  const files = new Map<string, Stamp>();
  const pending = [application];
  while (pending.length) {
    const path = pending.pop()!;
    if (files.has(path)) continue;
    if (files.size >= 50_000)
      throw new InstallationError(
        "application dependency tree exceeds its bound",
      );
    const info = lstatSync(path, { bigint: true });
    if ((info.mode & 0o022n) !== 0n && !info.isSymbolicLink())
      throw new InstallationError(
        "application dependencies must not be writable by other accounts",
      );
    if (
      process.getuid &&
      info.uid !== 0n &&
      info.uid !== BigInt(process.getuid())
    )
      throw new InstallationError(
        "application dependencies have an unexpected owner",
      );
    files.set(path, [
      info.dev,
      info.ino,
      info.size,
      info.mtimeNs,
      info.ctimeNs,
      info.mode,
    ]);
    if (info.isSymbolicLink()) {
      const target = realpathSync(path);
      if (!within(target, application))
        throw new InstallationError(
          "application dependency links outside the selected installation",
        );
      pending.push(target);
    } else if (info.isDirectory()) {
      for (const child of readdirSync(path)) pending.push(join(path, child));
    } else if (!info.isFile())
      throw new InstallationError(
        "application contains an unsupported dependency entry",
      );
  }
  return files;
}

/** Captures the installed dependency; it cannot silently follow an application update. */
export class Installation {
  readonly application: string;
  readonly resources: string;
  readonly runtime: string;
  readonly node: string;
  readonly nodeRepl: string;
  readonly entry: string;
  readonly codex: string;
  readonly modules: string;
  readonly version: string;
  readonly runtimeVersion: string;
  readonly platform: "darwin" | "linux";
  readonly arch: "arm64" | "x64";
  readonly native?: {
    service: string;
    client: string;
    helper: string;
    bundle: string;
  };
  readonly #files: ReadonlyMap<string, Stamp>;

  private constructor(value: {
    application: string;
    resources: string;
    runtime: string;
    node: string;
    nodeRepl: string;
    entry: string;
    codex: string;
    modules: string;
    version: string;
    runtimeVersion: string;
    platform: "darwin" | "linux";
    arch: "arm64" | "x64";
    native?: {
      service: string;
      client: string;
      helper: string;
      bundle: string;
    };
    files: ReadonlyMap<string, Stamp>;
  }) {
    this.application = value.application;
    this.resources = value.resources;
    this.runtime = value.runtime;
    this.node = value.node;
    this.nodeRepl = value.nodeRepl;
    this.entry = value.entry;
    this.codex = value.codex;
    this.modules = value.modules;
    this.version = value.version;
    this.runtimeVersion = value.runtimeVersion;
    this.platform = value.platform;
    this.arch = value.arch;
    this.native = value.native;
    this.#files = value.files;
  }

  static async inspect(
    path: string,
    signal: AbortSignal,
  ): Promise<Installation> {
    signal.throwIfAborted();
    const platform = process.platform;
    const arch = process.arch;
    if (!(
      (platform === "linux" && (arch === "arm64" || arch === "x64")) ||
      (platform === "darwin" && arch === "arm64")
    ))
      throw new InstallationError(
        "direct CUA supports macOS arm64 and Linux arm64/x64 with X11",
      );
    if (!isAbsolute(path))
      throw new InstallationError("CUA application path must be absolute");
    try {
      const application = realpathSync(path);
      if (!statSync(application).isDirectory())
        throw new Error("application is not a directory");
      const files = capture(application);
      const resources = join(
        application,
        platform === "darwin" ? "Contents/Resources" : "resources",
      );
      const runtime = join(resources, "cua_node");
      const manifestPath = regular(application, join(runtime, "manifest.json"));
      const manifest = manifestSchema.parse(
        JSON.parse(readBounded(manifestPath).toString("utf8")),
      );
      if (manifest.platform !== platform || manifest.arch !== arch)
        throw new InstallationError(
          "CUA runtime platform or architecture differs from the Runtime host",
        );
      const node = regular(
        application,
        join(runtime, manifest.node_path),
        true,
      );
      const nodeRepl = regular(
        application,
        join(runtime, manifest.node_repl_path),
        true,
      );
      const modules = realpathSync(join(runtime, manifest.node_modules));
      if (!within(modules, application))
        throw new InstallationError(
          "CUA modules leave the selected application",
        );
      const entry = regular(
        application,
        join(modules, "@oai/cua-repl/bin/cua-repl.mjs"),
      );
      const sky = regular(
        application,
        join(modules, "@oai/sky/dist/project/cua/sky_js/src/service.js"),
      );
      const tools: { codex: string; host: string }[] = [];
      for (const directory of [resources, join(resources, "codex-cli/bin")]) {
        try {
          const codex = regular(application, join(directory, "codex"), true);
          const host = regular(
            application,
            join(directory, "codex-code-mode-host"),
            true,
          );
          tools.push({ codex, host });
        } catch (cause) {
          if (
            !(cause instanceof InstallationError) ||
            !(
              (cause.cause as NodeJS.ErrnoException | undefined)?.code ===
              "ENOENT"
            )
          )
            throw cause;
        }
      }
      if (tools.length !== 1)
        throw new InstallationError(
          "application must contain one complete Codex tool layout",
        );
      const toolsEntry = tools[0]!;
      const originalDeclaration = regular(
        application,
        join(
          resources,
          "plugins/openai-bundled/plugins/unified-computer-use/.mcp.json",
        ),
      );
      const declared = JSON.parse(
        readBounded(originalDeclaration).toString("utf8"),
      );
      if (
        !["js", "js_reset", "turn_ended"].every((name) =>
          declared?.mcpServers?.cua_repl?.enabled_tools?.includes(name),
        )
      )
        throw new InstallationError(
          "application does not declare the required CUA control surface",
        );
      const archive = regular(application, join(resources, "app.asar"));
      const metadata = appMetadata(archive);
      let native: Installation["native"];
      if (platform === "darwin") {
        const bundle = join(modules, "@oai/sky/Codex Computer Use.app");
        const client = regular(
          application,
          join(
            modules,
            "@oai/sky/dist/project/cua/sky_js/src/targets/mac/client.js",
          ),
        );
        const helper = regular(
          application,
          join(
            bundle,
            "Contents/SharedSupport/SkyComputerUseClient.app/Contents/MacOS/SkyComputerUseClient",
          ),
          true,
        );
        regular(application, join(application, "Contents/Info.plist"));
        regular(
          application,
          join(application, "Contents/_CodeSignature/CodeResources"),
        );
        await verifyBundle(application, "com.openai.codex", signal);
        await verifyBundle(bundle, "com.openai.sky.CUAService", signal);
        native = { service: sky, client, helper, bundle };
      }
      signal.throwIfAborted();
      const installation = new Installation({
        application,
        resources,
        runtime,
        node,
        nodeRepl,
        modules,
        entry,
        codex: toolsEntry.codex,
        version: metadata.version,
        runtimeVersion: manifest.runtime_archive_version,
        platform,
        arch,
        native,
        files,
      });
      installation.assertCurrent();
      return installation;
    } catch (cause) {
      if (cause instanceof InstallationError) throw cause;
      throw new InstallationError(
        "selected application is not a complete compatible CUA installation",
        cause,
      );
    }
  }

  assertCurrent(): void {
    try {
      const current = capture(this.application);
      if (
        current.size !== this.#files.size ||
        [...this.#files].some(([path, stamp]) => {
          const observed = current.get(path);
          return (
            !observed || stamp.some((value, index) => value !== observed[index])
          );
        })
      )
        throw new Error("selected files changed");
    } catch (cause) {
      throw new InstallationError(
        "CUA application changed; close this lease and open a new one without replaying earlier actions",
        cause,
      );
    }
  }
}
