import { constants } from "node:fs";
import { access, open, readdir, realpath, stat } from "node:fs/promises";
import { createHash } from "node:crypto";
import { basename, dirname, isAbsolute, join, resolve, sep } from "node:path";
import { z } from "zod";

interface BrowserApplication {
  resources: string;
  node: string;
  nodeRepl: string;
  codex: string;
  platform: "darwin" | "linux";
  arch: "arm64" | "x64";
}

const boundedPath = z
  .string()
  .min(1)
  .max(8192)
  .refine((value) => !value.includes("\0"));
const paths = z.object({
  nativeMessagingManifestDirectories: z.array(boundedPath).min(1).max(32),
});
const metadata = z.object({
  extensionHostName: z.string().regex(/^[a-z0-9_.-]+$/),
  extensionIds: z
    .array(z.string().regex(/^[a-p]{32}$/))
    .min(1)
    .max(32),
  browserDiagnostics: z
    .array(
      z.object({
        browserFamily: z.string().regex(/^[a-z0-9-]+$/),
        linux: paths,
        macos: paths,
      }),
    )
    .min(1)
    .max(32),
});
const manifest = z.object({
  name: z.string(),
  type: z.literal("stdio"),
  path: boundedPath.refine(isAbsolute),
  allowed_origins: z.array(z.string()).min(1).max(32),
});
const hostConfiguration = z.strictObject({
  schemaVersion: z.literal(1),
  channel: z.string().min(1).optional(),
  browserClientPath: boundedPath.refine(isAbsolute),
  codexCliPath: boundedPath.refine(isAbsolute),
  nodePath: boundedPath.refine(isAbsolute),
  nodeReplPath: boundedPath.refine(isAbsolute),
  proxyHost: z.literal("127.0.0.1"),
  proxyPort: z.number().int().min(0).max(65535),
});
const MAX_JSON_BYTES = 256 * 1024;
const MAX_FILE_BYTES = 64 * 1024 * 1024;
const MAX_TREE_BYTES = 256 * 1024 * 1024;
const MAX_TREE_ENTRIES = 10_000;

async function read(
  path: string,
  signal: AbortSignal,
  maximum: number,
  consume: (bytes: Uint8Array) => void,
): Promise<number> {
  signal.throwIfAborted();
  const file = await open(path, constants.O_RDONLY | constants.O_NONBLOCK);
  try {
    const info = await file.stat();
    if (!info.isFile() || info.size > maximum)
      throw new Error(
        "computer: browser connector resource exceeds its file bound",
      );
    const buffer = Buffer.alloc(Math.min(maximum + 1, 64 * 1024));
    let total = 0;
    for (;;) {
      signal.throwIfAborted();
      const { bytesRead } = await file.read(buffer);
      if (!bytesRead) return total;
      total += bytesRead;
      if (total > maximum)
        throw new Error(
          "computer: browser connector resource exceeds its file bound",
        );
      consume(buffer.subarray(0, bytesRead));
    }
  } finally {
    await file.close();
  }
}

async function json(path: string, signal: AbortSignal): Promise<unknown> {
  const chunks: Buffer[] = [];
  await read(path, signal, MAX_JSON_BYTES, (bytes) =>
    chunks.push(Buffer.from(bytes)),
  );
  return JSON.parse(
    new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks)),
  );
}

async function sameFile(
  left: string,
  right: string,
  signal: AbortSignal,
): Promise<boolean> {
  if ((await realpath(left)) === (await realpath(right))) return true;
  const a = createHash("sha256");
  await read(left, signal, MAX_FILE_BYTES, (bytes) => {
    a.update(bytes);
  });
  const b = createHash("sha256");
  await read(right, signal, MAX_FILE_BYTES, (bytes) => {
    b.update(bytes);
  });
  return a.digest("hex") === b.digest("hex");
}

async function treeDigest(root: string, signal: AbortSignal): Promise<string> {
  const hash = createHash("sha256");
  let total = 0;
  let entries = 0;
  async function visit(directory: string, prefix: string): Promise<void> {
    const children = await readdir(directory, { withFileTypes: true });
    children.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
    for (const child of children) {
      signal.throwIfAborted();
      if (++entries > MAX_TREE_ENTRIES)
        throw new Error(
          "computer: browser connector resource tree exceeds its entry bound",
        );
      const name = `${prefix}${child.name}`;
      // The original installer creates this configuration; its selected paths are checked separately.
      if (
        /^extension-host\/[^/]+\/[^/]+\/extension-host-config\.json$/.test(name)
      )
        continue;
      if (child.isDirectory()) {
        hash.update(`directory\0${name}\0`);
        await visit(join(directory, child.name), `${name}/`);
      } else if (child.isFile()) {
        const digest = createHash("sha256");
        total += await read(
          join(directory, child.name),
          signal,
          Math.min(MAX_FILE_BYTES, MAX_TREE_BYTES - total),
          (bytes) => {
            digest.update(bytes);
          },
        );
        hash.update(`file\0${name}\0${digest.digest("hex")}\0`);
      } else {
        throw new Error(
          "computer: browser connector copy contains an unsupported resource",
        );
      }
    }
  }
  await visit(root, "");
  return hash.digest("hex");
}

function relativeDirectory(value: string): string {
  if (
    isAbsolute(value) ||
    value
      .split(/[\\/]/)
      .some((part) => part === ".." || part === "." || part === "")
  )
    throw new Error(
      "computer: original browser connector metadata has an invalid directory",
    );
  return value;
}

function manifestPaths(
  information: z.infer<typeof metadata>,
  platform: BrowserApplication["platform"],
  environment: Readonly<Record<string, string>>,
): string[] {
  const home = environment.HOME;
  if (!home || !isAbsolute(home))
    throw new Error(
      "computer: browser connector requires the desktop account home",
    );
  const result = new Set<string>();
  for (const browser of information.browserDiagnostics) {
    const directories =
      browser[platform === "darwin" ? "macos" : "linux"]
        .nativeMessagingManifestDirectories;
    for (const entry of directories) {
      const relative = relativeDirectory(entry);
      let root = home;
      let directory = relative;
      if (platform === "linux") {
        if (!relative.startsWith(".config/"))
          throw new Error(
            "computer: original browser connector metadata has an unsupported config root",
          );
        root =
          (browser.browserFamily === "chrome"
            ? environment.CHROME_CONFIG_HOME
            : undefined) ||
          environment.XDG_CONFIG_HOME ||
          join(home, ".config");
        if (!isAbsolute(root))
          throw new Error("computer: browser config root must be absolute");
        directory = relative.slice(".config/".length);
      }
      result.add(
        join(root, directory, `${information.extensionHostName}.json`),
      );
    }
  }
  return [...result];
}

function absent(cause: unknown): boolean {
  return (
    cause instanceof Error &&
    "code" in cause &&
    (cause.code === "ENOENT" || cause.code === "ENOTDIR")
  );
}

/** Inspect original registration only. The engine remains the owner of live connections and site approval. */
export async function validateBrowser(
  application: BrowserApplication,
  environment: Readonly<Record<string, string>>,
  signal: AbortSignal,
): Promise<void> {
  const plugin = join(
    application.resources,
    "plugins/openai-bundled/plugins/chrome",
  );
  let information: z.infer<typeof metadata>;
  try {
    information = metadata.parse(
      await json(join(plugin, "scripts/extension-ids.json"), signal),
    );
  } catch (cause) {
    throw new Error(
      "computer: selected CUA application is missing its original browser connector metadata",
      { cause },
    );
  }
  const sourceHost = join(
    plugin,
    "extension-host",
    application.platform === "darwin" ? "macos" : "linux",
    application.arch,
    application.platform === "darwin" ? "ChatGPT for Chrome" : "extension-host",
  );
  try {
    signal.throwIfAborted();
    if (!(await stat(sourceHost)).isFile())
      throw new Error("computer: original browser host is not a regular file");
    await access(sourceHost, constants.X_OK);
  } catch (cause) {
    throw new Error(
      "computer: selected CUA application is missing its executable original browser host",
      { cause },
    );
  }
  const sourceClient = await realpath(
    join(plugin, "scripts/browser-client.mjs"),
  );
  const sourceRoot = await realpath(plugin);
  const acceptedHosts = new Set<string>();
  const acceptedRoots = new Set<string>([sourceRoot]);
  let sourceDigest: string | undefined;
  let found = false;
  for (const path of manifestPaths(
    information,
    application.platform,
    environment,
  )) {
    let value: unknown;
    try {
      value = await json(path, signal);
    } catch (cause) {
      if (absent(cause)) continue;
      throw new Error(
        "computer: browser connector registration could not be inspected",
        { cause },
      );
    }
    let registered: z.infer<typeof manifest>;
    try {
      registered = manifest.parse(value);
      const origins = new Set(registered.allowed_origins);
      if (
        registered.name !== information.extensionHostName ||
        origins.size !== information.extensionIds.length ||
        information.extensionIds.some(
          (id) => !origins.has(`chrome-extension://${id}/`),
        )
      )
        throw new Error(
          "computer: browser connector identity differs from the selected application",
        );
      const target = await realpath(registered.path);
      await access(target, constants.X_OK);
      if (acceptedHosts.has(target)) {
        found = true;
        continue;
      }
      if (!(await sameFile(target, sourceHost, signal)))
        throw new Error(
          "computer: browser connector is not the original selected CUA host",
        );
      const config = hostConfiguration.parse(
        await json(join(dirname(target), "extension-host-config.json"), signal),
      );
      for (const [configured, selected] of [
        [config.nodePath, application.node],
        [config.nodeReplPath, application.nodeRepl],
        [config.codexCliPath, application.codex],
      ]) {
        if ((await realpath(configured!)) !== (await realpath(selected!)))
          throw new Error(
            "computer: browser connector selects a different CUA installation",
          );
      }
      const client = await realpath(config.browserClientPath);
      if (client !== sourceClient) {
        if (
          basename(client) !== "browser-client.mjs" ||
          basename(dirname(client)) !== "scripts"
        )
          throw new Error(
            "computer: browser connector has an unsupported client layout",
          );
        const root = dirname(dirname(client));
        if (!acceptedRoots.has(root)) {
          sourceDigest ??= await treeDigest(sourceRoot, signal);
          if ((await treeDigest(root, signal)) !== sourceDigest)
            throw new Error(
              "computer: browser connector copy differs from the selected CUA resources",
            );
          acceptedRoots.add(root);
        }
      }
      // A copied host and client must belong to the same intact connector resources.
      const clientRoot = dirname(dirname(client));
      if (
        target !== (await realpath(sourceHost)) &&
        !target.startsWith(resolve(clientRoot, "extension-host") + sep)
      )
        throw new Error(
          "computer: browser connector host and client select different resources",
        );
      acceptedHosts.add(target);
      found = true;
    } catch (cause) {
      throw new Error(
        "computer: browser connector is incompatible; restore the original connector through the selected ChatGPT application before enabling browser control",
        { cause },
      );
    }
  }
  if (!found)
    throw new Error(
      "computer: configure the official ChatGPT browser extension and native connector for this desktop account before enabling browser control",
    );
}
