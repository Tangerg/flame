import { cp, mkdir, readFile, writeFile } from "node:fs/promises";
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
  for (const feature of ["memory", "schedules"]) {
    const target = join(output, feature);
    await mkdir(join(target, "bin"), { recursive: true });
    await cp(join(source, feature, "views"), join(target, "views"), {
      recursive: true,
    });
    await cp(join(source, feature, "plugin.json"), join(target, "plugin.json"));
    const descriptor = JSON.parse(
      await readFile(join(source, feature, "mcp.json"), "utf8"),
    );
    const executable = feature + (os === "windows" ? ".exe" : "");
    descriptor.mcpServers[feature].command = `./bin/${executable}`;
    await writeFile(
      join(target, "mcp.json"),
      JSON.stringify(descriptor, null, 2) + "\n",
    );
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
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const { values } = parseArgs({
    options: {
      os: { type: "string" },
      arch: { type: "string" },
      output: { type: "string" },
    },
  });
  await buildPlugins(values);
}
