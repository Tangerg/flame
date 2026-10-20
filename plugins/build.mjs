import {
  cp,
  mkdir,
  readFile,
  writeFile,
  readdir,
  access,
  mkdtemp,
  rename,
  rm,
  realpath,
} from "node:fs/promises";
import { spawn } from "node:child_process";
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
import { parseArgs } from "node:util";

const source = await realpath(dirname(fileURLToPath(import.meta.url)));

async function physicalDestination(path) {
  try {
    return await realpath(path);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
    return join(await physicalDestination(dirname(path)), basename(path));
  }
}

async function validateDestination(path) {
  if (typeof path !== "string" || !isAbsolute(path))
    throw new Error("output must be a new absolute directory");
  const destination = await physicalDestination(resolve(path));
  for (const [parent, child] of [
    [source, destination],
    [destination, source],
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

function go(args, env) {
  return new Promise((resolve, reject) => {
    const child = spawn("go", args, { cwd: source, env, stdio: "inherit" });
    child.on("error", reject);
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(`go exited ${code}`)),
    );
  });
}

export async function buildPlugins({ os, arch, output, plugin, archives }) {
  if (
    !["darwin", "linux", "windows"].includes(os) ||
    !["amd64", "arm64"].includes(arch)
  )
    throw new Error("a supported target OS and architecture are required");
  await validateDestination(output);
  if (archives) {
    await validateDestination(archives);
    await mkdir(archives);
  }
  if (plugin && !/^[a-z][a-z0-9-]*$/.test(plugin))
    throw new Error("invalid plugin directory name");
  let singlePackage = false;
  try {
    await access(join(source, "plugin.json"));
    singlePackage = true;
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (plugin && singlePackage)
    throw new Error("single-package author projects do not accept --plugin");
  if (plugin && !singlePackage)
    await access(join(source, plugin, "plugin.json"));
  await mkdir(output);
  const entries = singlePackage
    ? [
        {
          name: JSON.parse(await readFile(join(source, "plugin.json"), "utf8"))
            .name,
          isDirectory: () => true,
        },
      ]
    : await readdir(source, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isDirectory() || (plugin && entry.name !== plugin)) continue;
    const feature = entry.name;
    const packageSource = singlePackage ? source : join(source, feature);
    const manifestPath = join(packageSource, "plugin.json");
    try {
      await access(manifestPath);
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    if (!/^[a-zA-Z0-9][a-zA-Z0-9.-]{0,127}$/.test(feature))
      throw new Error("invalid package directory name");
    const target = join(output, feature);
    await mkdir(target);
    await cp(manifestPath, join(target, "plugin.json"));
    const namespace = "io.github.tangerg.flame";
    if ((await readdir(packageSource)).includes(namespace))
      await cp(join(packageSource, namespace), join(target, namespace), {
        recursive: true,
      });
    if ((await readdir(packageSource)).includes("skills"))
      await cp(join(packageSource, "skills"), join(target, "skills"), {
        recursive: true,
      });
    try {
      await access(join(packageSource, "mcp.json"));
    } catch (error) {
      if (error.code === "ENOENT") {
        if (archives)
          await archivePlugin(target, manifestPath, os, arch, archives);
        continue;
      }
      throw error;
    }
    const descriptor = JSON.parse(
      await readFile(join(packageSource, "mcp.json"), "utf8"),
    );
    await mkdir(join(target, "bin"));
    for (const [name, server] of Object.entries(descriptor.mcpServers)) {
      if (!server.command) continue;
      const executable = name + (os === "windows" ? ".exe" : "");
      server.command = `./bin/${executable}`;
      await go(
        [
          "build",
          "-trimpath",
          "-ldflags=-s -w",
          "-o",
          join(target, "bin", executable),
          singlePackage ? "./backend" : `./${feature}/backend`,
        ],
        {
          ...process.env,
          CGO_ENABLED: "0",
          GOOS: os,
          GOARCH: arch,
        },
      );
    }
    await writeFile(
      join(target, "mcp.json"),
      JSON.stringify(descriptor, null, 2) + "\n",
    );
    if (archives) await archivePlugin(target, manifestPath, os, arch, archives);
  }
}

async function archivePlugin(target, manifestPath, os, arch, archives) {
  const { name, version } = JSON.parse(await readFile(manifestPath, "utf8"));
  for (const value of [name, version]) {
    if (
      typeof value !== "string" ||
      !/^[a-zA-Z0-9][a-zA-Z0-9.+-]{0,127}$/.test(value)
    )
      throw new Error("plugin archive requires a bounded name and version");
  }
  await go(
    [
      "run",
      "./cmd/archive",
      "--source",
      target,
      "--output",
      join(archives, `plugin-${name}-${version}-${os}-${arch}.zip`),
    ],
    { ...process.env, GOOS: "", GOARCH: "" },
  );
}

if (
  process.argv[1] &&
  (await realpath(process.argv[1])) ===
    (await realpath(fileURLToPath(import.meta.url)))
) {
  const { values } = parseArgs({
    options: {
      os: { type: "string" },
      arch: { type: "string" },
      output: { type: "string" },
      plugin: { type: "string" },
      archives: { type: "string" },
      replace: { type: "boolean", default: false },
    },
  });
  if (!values.replace) await buildPlugins(values);
  else {
    await validateDestination(values.output);
    await mkdir(dirname(values.output), { recursive: true });
    const stage = await mkdtemp(join(dirname(values.output), ".first-party-"));
    try {
      const built = join(stage, "plugins");
      await buildPlugins({ ...values, output: built });
      await rm(values.output, { recursive: true, force: true });
      await rename(built, values.output);
    } finally {
      await rm(stage, { recursive: true, force: true });
    }
  }
}
