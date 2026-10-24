import {
  access,
  cp,
  mkdir,
  readFile,
  realpath,
  writeFile,
} from "node:fs/promises";
import { execFile } from "node:child_process";
import {
  basename,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs, promisify } from "node:util";

const run = promisify(execFile);
async function physical(path) {
  try {
    return await realpath(path);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
    return join(await physical(dirname(path)), basename(path));
  }
}
export async function validateDestination(source, output) {
  if (!isAbsolute(source) || !isAbsolute(output))
    throw new Error("source and output must be absolute directories");
  const from = await realpath(source),
    to = await physical(output);
  for (const [parent, child] of [
    [from, to],
    [to, from],
  ]) {
    const difference = relative(parent, child);
    if (
      difference === "" ||
      (difference !== ".." &&
        !difference.startsWith(`..${sep}`) &&
        !isAbsolute(difference))
    )
      throw new Error("build destinations must be outside the plugin source");
  }
}
async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}
function resourcePath(value) {
  if (
    typeof value !== "string" ||
    value.includes("\\") ||
    isAbsolute(value) ||
    value.split("/").some((part) => !part || part === "." || part === "..")
  )
    throw new Error("plugin build requires a package-relative resource path");
  return value;
}
export async function readBuildPlan(source) {
  const resources = join(source, "package");
  const manifest = JSON.parse(
    await readFile(join(resources, "plugin.json"), "utf8"),
  );
  const reference =
    manifest.extensions?.["io.github.tangerg.flame"]?.declaration;
  const declaration =
    reference === undefined
      ? {}
      : JSON.parse(
          await readFile(join(resources, resourcePath(reference)), "utf8"),
        );
  const mcpPath = join(resources, "mcp.json");
  const mcp = (await exists(mcpPath))
    ? JSON.parse(await readFile(mcpPath, "utf8"))
    : undefined;
  const managed = declaration.managedBackends ?? [];
  if (!Array.isArray(managed))
    throw new Error("managed backends must be an array");
  const ids = new Set(Object.keys(mcp?.mcpServers ?? {}));
  for (const backend of managed) {
    if (
      typeof backend.id !== "string" ||
      !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(backend.id) ||
      ids.has(backend.id) ||
      backend.runtime !== "node" ||
      !resourcePath(backend.entry).endsWith(".mjs") ||
      Object.keys(backend).some(
        (key) => !["id", "runtime", "entry"].includes(key),
      )
    )
      throw new Error(
        "invalid or duplicate managed backend startup declaration",
      );
    ids.add(backend.id);
  }
  return {
    manifest,
    declaration,
    mcp,
    managed,
    native: Object.values(mcp?.mcpServers ?? {}).some(
      (server) => server.command !== undefined,
    ),
  };
}
export async function buildPlugin({
  source,
  output,
  os,
  arch,
  archives,
  archiveTool,
}) {
  await validateDestination(source, output);
  const plan = await readBuildPlan(source);
  if (
    plan.native &&
    (!["darwin", "linux", "windows"].includes(os) ||
      !["amd64", "arm64"].includes(arch))
  )
    throw new Error(
      "native backends require a supported target OS and architecture",
    );
  const resources = join(source, "package");
  const { manifest } = plan;
  await mkdir(output);
  await cp(resources, output, { recursive: true });
  const mcp = join(output, "mcp.json");
  if (plan.mcp) {
    const descriptor = plan.mcp;
    await mkdir(join(output, "bin"));
    for (const [name, server] of Object.entries(descriptor.mcpServers)) {
      if (!server.command) continue;
      if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(name))
        throw new Error("invalid backend executable name");
      const executable = name + (os === "windows" ? ".exe" : "");
      await run(
        "go",
        [
          "build",
          "-trimpath",
          "-ldflags=-s -w",
          "-o",
          join(output, "bin", executable),
          "./backend",
        ],
        {
          cwd: source,
          env: { ...process.env, CGO_ENABLED: "0", GOOS: os, GOARCH: arch },
          maxBuffer: 32 * 1024 * 1024,
        },
      );
      server.command = `./bin/${executable}`;
    }
    await writeFile(mcp, JSON.stringify(descriptor, null, 2) + "\n");
  }
  if (plan.managed.length) {
    const { buildBackend } = await import("@flame/backend-sdk/build");
    const { rolldown } = await import("rolldown");
    for (const backend of plan.managed)
      await buildBackend({
        compiler: rolldown,
        input: join(source, backend.entry.slice(0, -4) + ".ts"),
        directory: dirname(join(output, backend.entry)),
        entryFileName: basename(backend.entry),
      });
  }
  if (archives) {
    await validateDestination(source, archives);
    await mkdir(archives, { recursive: true });
    for (const value of [manifest.name, manifest.version])
      if (
        typeof value !== "string" ||
        !/^[a-zA-Z0-9][a-zA-Z0-9.+-]{0,127}$/.test(value)
      )
        throw new Error("plugin archive requires a bounded name and version");
    const archiveArgs = [
      "--source",
      output,
      "--output",
      join(
        archives,
        `plugin-${manifest.name}-${manifest.version}${plan.native ? `-${os}-${arch}` : ""}.zip`,
      ),
    ];
    if (archiveTool !== undefined && !isAbsolute(archiveTool))
      throw new Error("archive tool must be an absolute executable path");
    if (plan.managed.length && !archiveTool)
      throw new Error(
        "managed backend packaging requires an explicit prebuilt archive tool",
      );
    await run(
      archiveTool ?? "go",
      archiveTool
        ? archiveArgs
        : [
            "tool",
            "github.com/Tangerg/flame/plugins/scripts/archive",
            ...archiveArgs,
          ],
      {
        cwd: source,
        env: { ...process.env, GOOS: "", GOARCH: "" },
        maxBuffer: 32 * 1024 * 1024,
      },
    );
  }
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  const { values } = parseArgs({
    options: {
      source: { type: "string" },
      output: { type: "string" },
      os: { type: "string" },
      arch: { type: "string" },
      archives: { type: "string" },
      "archive-tool": { type: "string" },
    },
  });
  if (!values.source || !values.output)
    throw new Error("--source and --output are required");
  await buildPlugin({ ...values, archiveTool: values["archive-tool"] });
}
