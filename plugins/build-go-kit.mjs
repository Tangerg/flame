import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const runtimeModule = "github.com/Tangerg/flame/runtime";
const localModule = `${runtimeModule}/localruntime`;

function run(args, cwd, env) {
  return execFileSync("go", args, {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 32 * 1024 * 1024,
  });
}

function proxyPath(module) {
  return module.replace(/[A-Z]/g, (character) => `!${character.toLowerCase()}`);
}

// Restrict snapshots to compiler input; application configuration can contain credentials.
async function snapshot(directory, destination, module, localVersion) {
  await mkdir(destination);
  const hash = createHash("sha256");
  async function copy(relative = "") {
    const entries = await readdir(join(directory, relative), {
      withFileTypes: true,
    });
    entries.sort((a, b) => a.name.localeCompare(b.name, "en"));
    for (const entry of entries) {
      const resource = relative ? `${relative}/${entry.name}` : entry.name;
      if (entry.isDirectory()) {
        if (
          [
            "node_modules",
            "vendor",
            "build",
            "dist",
            "testdata",
            ".git",
          ].includes(entry.name) ||
          resource === "internal/testsupport" ||
          (module === runtimeModule && resource === "localruntime")
        )
          continue;
        await mkdir(join(destination, resource));
        await copy(resource);
        continue;
      }
      if (
        !entry.isFile() ||
        (entry.name !== "go.mod" &&
          entry.name !== "go.sum" &&
          (!entry.name.endsWith(".go") || entry.name.endsWith("_test.go")))
      )
        continue;
      let content = await readFile(join(directory, resource));
      if (resource === "go.mod" && localVersion) {
        const text = content
          .toString()
          .replace(
            /github\.com\/Tangerg\/flame\/runtime\/localruntime\s+\S+/,
            `${localModule} ${localVersion}`,
          );
        content = Buffer.from(text);
      }
      hash.update(resource).update("\0").update(content).update("\0");
      await writeFile(join(destination, resource), content);
    }
  }
  await copy();
  return `v0.0.0-source.${hash.digest("hex")}`;
}

export async function prepareGoKit({ repository, output, offline }) {
  const artifacts = join(output, "artifacts", "go");
  const sources = join(artifacts, "sources");
  const proxy = join(artifacts, "proxy");
  await mkdir(sources, { recursive: true });
  const localSource = join(sources, "localruntime");
  const localVersion = await snapshot(
    join(repository, "runtime", "localruntime"),
    localSource,
    localModule,
  );
  const runtimeSource = join(sources, "runtime");
  const runtimeVersion = await snapshot(
    join(repository, "runtime"),
    runtimeSource,
    runtimeModule,
    localVersion,
  );
  for (const [module, version, source] of [
    [localModule, localVersion, localSource],
    [runtimeModule, runtimeVersion, runtimeSource],
  ]) {
    const directory = join(proxy, proxyPath(module), "@v");
    await mkdir(directory, { recursive: true });
    await writeFile(
      join(directory, `${version}.mod`),
      await readFile(join(source, "go.mod")),
    );
    await writeFile(
      join(directory, `${version}.info`),
      JSON.stringify({ Version: version, Time: "1970-01-01T00:00:00Z" }),
    );
    run(
      [
        "run",
        "./cmd/archive",
        "--source",
        source,
        "--output",
        join(directory, `${version}.zip`),
        "--prefix",
        `${module}@${version}`,
      ],
      join(repository, "plugins"),
      { ...process.env, GOOS: "", GOARCH: "" },
    );
  }
  const env = {
    ...process.env,
    GOWORK: "off",
    GOPROXY: `${pathToFileURL(proxy).href},${offline ? "off" : "https://proxy.golang.org,direct"}`,
    GONOSUMDB: [process.env.GONOSUMDB, runtimeModule, localModule]
      .filter(Boolean)
      .join(","),
  };
  run(
    ["mod", "edit", "-require", `${runtimeModule}@${runtimeVersion}`],
    output,
    env,
  );
  run(["mod", "tidy"], output, env);
  run(["mod", "vendor"], output, env);
  await writeFile(
    join(output, "vendor-go.mjs"),
    `import { execFileSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
const root = fileURLToPath(new URL("./", import.meta.url));
const env = { ...process.env, GOWORK: "off", GOPROXY: pathToFileURL(fileURLToPath(new URL("./artifacts/go/proxy", import.meta.url))).href + ",https://proxy.golang.org,direct", GONOSUMDB: "${runtimeModule},${localModule}" };
execFileSync("go", ["mod", "tidy"], { cwd: root, env, stdio: "inherit" });
execFileSync("go", ["mod", "vendor"], { cwd: root, env, stdio: "inherit" });
`,
  );
  run(["build", "-trimpath", "./..."], output, {
    ...env,
    GOPROXY: "off",
    GOSUMDB: "off",
  });
}
