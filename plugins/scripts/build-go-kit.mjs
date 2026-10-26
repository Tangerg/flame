import { execFileSync } from "node:child_process";
import { writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { pathToFileURL } from "node:url";

const sdkPath = "github.com/Tangerg/flame/packages/go-sdk";
export function authorSDK(artifacts) {
  const version =
    artifacts.members["github.com/Tangerg/flame/plugins"]?.requires[sdkPath];
  const artifact = artifacts.modules[`${sdkPath}@${version}`];
  if (!artifact)
    throw new Error(
      "the frozen plugins module must select a prepared Go SDK artifact",
    );
  return artifact;
}
function environment(repository, artifacts, offline) {
  const cache = execFileSync("go", ["env", "GOMODCACHE"], {
    cwd: repository,
    encoding: "utf8",
  }).trim();
  return {
    ...process.env,
    GOWORK: "off",
    GOTOOLCHAIN: "local",
    GONOPROXY: "none",
    GONOSUMDB: "github.com/Tangerg/flame",
    GOPROXY: [
      pathToFileURL(artifacts.proxy).href,
      pathToFileURL(join(cache, "cache/download")).href,
      offline ? "off" : "https://proxy.golang.org",
    ].join(","),
  };
}
async function writeModule(repository, directory, sdk, modulePath, tools = []) {
  await writeFile(
    join(directory, "go.mod"),
    `module ${modulePath}\n\ngo ${sdk.go}\n\nrequire ${sdk.path} ${sdk.version}\n` +
      (tools.length
        ? `\ntool (\n${tools.map((name) => "\t" + name).join("\n")}\n)\n`
        : ""),
  );
  await writeFile(
    join(directory, "go.sum"),
    execFileSync("git", ["show", `${sdk.revision}:${sdk.directory}/go.sum`], {
      cwd: repository,
    }),
  );
}
export async function prepareDevelopmentTools({
  repository,
  output,
  artifacts,
  offline,
}) {
  const source = await mkdtemp(join(tmpdir(), "flame-author-tools-"));
  try {
    await writeModule(
      repository,
      source,
      authorSDK(artifacts),
      "flame.invalid/author-tools",
    );
    const env = {
      ...environment(repository, artifacts, offline),
      GOOS: "",
      GOARCH: "",
    };
    await mkdir(join(output, "tools"));
    for (const name of ["contractgen", "archive"])
      execFileSync(
        "go",
        [
          "build",
          "-mod=mod",
          "-trimpath",
          "-o",
          join(
            output,
            "tools",
            name + (process.platform === "win32" ? ".exe" : ""),
          ),
          `${sdkPath}/cmd/${name}`,
        ],
        { cwd: source, env, stdio: "inherit" },
      );
  } finally {
    await rm(source, { recursive: true, force: true });
  }
}
export async function prepareGoKit({
  repository,
  output,
  artifacts,
  modulePath,
  offline,
  tools,
}) {
  const allowed = new Set([
    `${sdkPath}/cmd/contractgen`,
    `${sdkPath}/cmd/archive`,
  ]);
  for (const tool of tools)
    if (!allowed.has(tool))
      throw new Error(`author tool is not public: ${tool}`);
  await writeModule(
    repository,
    output,
    authorSDK(artifacts),
    modulePath,
    tools,
  );
  const env = environment(repository, artifacts, offline);
  const run = (args, selected = env) =>
    execFileSync("go", args, { cwd: output, env: selected, stdio: "inherit" });
  run(["mod", "tidy"]);
  run(["mod", "vendor"]);
  const vendorEnv = {
    ...env,
    GOPROXY: "off",
    GOSUMDB: "off",
    GOFLAGS: "-mod=vendor",
  };
  for (const command of ["generate", "test", "build"])
    run([command, "./..."], vendorEnv);
}
