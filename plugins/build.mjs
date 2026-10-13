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
} from "node:fs/promises";
import { spawn } from "node:child_process";
import { dirname, isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

const source = dirname(fileURLToPath(import.meta.url));
const repository = dirname(source);

function go(args, env) {
  return new Promise((resolve, reject) => {
    const child = spawn("go", args, { cwd: repository, env, stdio: "inherit" });
    child.on("error", reject);
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(`go exited ${code}`)),
    );
  });
}

export async function buildPlugins({ os, arch, output }) {
  if (
    !["darwin", "linux", "windows"].includes(os) ||
    !["amd64", "arm64"].includes(arch)
  )
    throw new Error("a supported target OS and architecture are required");
  if (!isAbsolute(output))
    throw new Error("output must be a new absolute directory");
  await mkdir(output);
  for (const entry of await readdir(source, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const feature = entry.name;
    const manifestPath = join(source, feature, "plugin.json");
    try {
      await access(manifestPath);
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    const target = join(output, feature);
    await mkdir(target);
    await cp(manifestPath, join(target, "plugin.json"));
    const namespace = "io.github.tangerg.flame";
    if ((await readdir(join(source, feature))).includes(namespace))
      await cp(join(source, feature, namespace), join(target, namespace), {
        recursive: true,
      });
    if ((await readdir(join(source, feature))).includes("skills"))
      await cp(join(source, feature, "skills"), join(target, "skills"), {
        recursive: true,
      });
    try {
      await access(join(source, feature, "mcp.json"));
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    const descriptor = JSON.parse(
      await readFile(join(source, feature, "mcp.json"), "utf8"),
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
          `./runtime/internal/plugins/${feature}`,
        ],
        {
          ...process.env,
          GOWORK: join(repository, "go.work"),
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
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const { values } = parseArgs({
    options: {
      os: { type: "string" },
      arch: { type: "string" },
      output: { type: "string" },
      replace: { type: "boolean", default: false },
    },
  });
  if (!values.replace) await buildPlugins(values);
  else {
    if (!isAbsolute(values.output)) throw new Error("output must be absolute");
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
