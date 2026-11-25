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
export function resourcePath(value) {
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
// Go builds and pluginpack can report large diagnostics; keep them whole.
const TOOL_OUTPUT_BYTES = 32 * 1024 * 1024;

async function buildNativeBackends(descriptor, { source, output, os, arch }) {
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
        maxBuffer: TOOL_OUTPUT_BYTES,
      },
    );
    server.command = `./bin/${executable}`;
  }
  await writeFile(
    join(output, "mcp.json"),
    JSON.stringify(descriptor, null, 2) + "\n",
  );
}

async function buildManagedBackends(backends, { source, output }) {
  const { default: protocol } = await import("@flame/backend-sdk/protocol", {
    with: { type: "json" },
  });
  const { buildBackend } = await import("@flame/backend-sdk/build");
  const { rolldown } = await import("rolldown");
  for (const backend of backends)
    await buildBackend({
      target: protocol.constants.nodeTarget,
      maximumFrameBytes: protocol.constants.maximumFrameBytes,
      compiler: rolldown,
      input: join(source, backend.entry.slice(0, -4) + ".ts"),
      directory: dirname(join(output, backend.entry)),
      entryFileName: basename(backend.entry),
    });
}

async function archiveArguments(plan, { source, archives, os, arch }) {
  if (!archives) return [];
  await validateDestination(source, archives);
  await mkdir(archives, { recursive: true });
  const { name, version } = plan.manifest;
  for (const value of [name, version])
    if (
      typeof value !== "string" ||
      !/^[a-zA-Z0-9][a-zA-Z0-9.+-]{0,127}$/.test(value)
    )
      throw new Error("plugin archive requires a bounded name and version");
  const target = plan.native ? `-${os}-${arch}` : "";
  return [
    "--archive",
    join(archives, `plugin-${name}-${version}${target}.zip`),
  ];
}

async function runPackageTool(packageTool, args, source) {
  if (packageTool !== undefined && !isAbsolute(packageTool))
    throw new Error("package tool must be an absolute executable path");
  const env = { ...process.env, GOOS: "", GOARCH: "" };
  const bundled = fileURLToPath(
    new URL(
      "./tools/pluginpack" + (process.platform === "win32" ? ".exe" : ""),
      import.meta.url,
    ),
  );
  if (packageTool || (await exists(bundled))) {
    await run(packageTool ?? bundled, args, {
      cwd: source,
      env,
      maxBuffer: TOOL_OUTPUT_BYTES,
    });
    return;
  }
  const runtimeModule = fileURLToPath(
    new URL("../../runtime/", import.meta.url),
  );
  if (!(await exists(join(runtimeModule, "cmd/pluginpack/main.go"))))
    throw new Error(
      "independent author builds require their prebuilt pluginpack tool",
    );
  await run("go", ["run", "./cmd/pluginpack", ...args], {
    cwd: runtimeModule,
    env,
    maxBuffer: TOOL_OUTPUT_BYTES,
  });
}

export async function buildPlugin(options) {
  const { source, output, os, arch, packageTool } = options;
  await validateDestination(source, output);
  const plan = await readBuildPlan(source);
  await mkdir(output);
  await cp(join(source, "package"), output, { recursive: true });
  if (plan.mcp) await buildNativeBackends(plan.mcp, options);
  if (plan.managed.length) await buildManagedBackends(plan.managed, options);
  const packageArgs = [
    "--source",
    output,
    ...(plan.native ? ["--os", os, "--arch", arch] : []),
    ...(await archiveArguments(plan, options)),
  ];
  await runPackageTool(packageTool, packageArgs, source);
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
      "package-tool": { type: "string" },
    },
  });
  if (!values.source || !values.output)
    throw new Error("--source and --output are required");
  await buildPlugin({ ...values, packageTool: values["package-tool"] });
}
