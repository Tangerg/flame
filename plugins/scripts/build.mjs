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
export async function buildPlugin({ source, output, os, arch, archives }) {
  if (
    !["darwin", "linux", "windows"].includes(os) ||
    !["amd64", "arm64"].includes(arch)
  )
    throw new Error("a supported target OS and architecture are required");
  await validateDestination(source, output);
  const resources = join(source, "package");
  const manifest = JSON.parse(
    await readFile(join(resources, "plugin.json"), "utf8"),
  );
  await mkdir(output);
  await cp(resources, output, { recursive: true });
  const mcp = join(output, "mcp.json");
  if (await exists(mcp)) {
    const descriptor = JSON.parse(await readFile(mcp, "utf8"));
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
  if (archives) {
    await validateDestination(source, archives);
    await mkdir(archives, { recursive: true });
    for (const value of [manifest.name, manifest.version])
      if (
        typeof value !== "string" ||
        !/^[a-zA-Z0-9][a-zA-Z0-9.+-]{0,127}$/.test(value)
      )
        throw new Error("plugin archive requires a bounded name and version");
    await run(
      "go",
      [
        "tool",
        "github.com/Tangerg/flame/plugins/scripts/archive",
        "--source",
        output,
        "--output",
        join(
          archives,
          `plugin-${manifest.name}-${manifest.version}-${os}-${arch}.zip`,
        ),
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
    },
  });
  if (!values.source || !values.output)
    throw new Error("--source and --output are required");
  await buildPlugin(values);
}
