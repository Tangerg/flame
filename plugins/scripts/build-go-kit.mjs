import { execFileSync } from "node:child_process";
import { cp, writeFile, mkdir, mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { pathToFileURL } from "node:url";
export async function prepareDevelopmentTools({
  repository,
  output,
  artifacts,
  offline,
}) {
  const source = await mkdtemp(join(tmpdir(), "flame-author-tools-"));
  try {
    const manifest = JSON.parse(
      execFileSync("go", ["mod", "edit", "-json"], { cwd: repository }),
    );
    await writeFile(
      join(source, "go.mod"),
      `module flame.invalid/author-tools\n\ngo ${manifest.Go}\n\nrequire github.com/Tangerg/flame ${artifacts.version}\n`,
    );
    await cp(join(repository, "go.sum"), join(source, "go.sum"));
    const cache = execFileSync("go", ["env", "GOMODCACHE"], {
      cwd: repository,
      encoding: "utf8",
    }).trim();
    const env = {
      ...process.env,
      GOWORK: "off",
      GOTOOLCHAIN: "local",
      GONOPROXY: "none",
      GONOSUMDB: "github.com/Tangerg/flame",
      GOOS: "",
      GOARCH: "",
      GOPROXY: [
        pathToFileURL(artifacts.proxy).href,
        pathToFileURL(join(cache, "cache/download")).href,
        offline ? "off" : "https://proxy.golang.org",
      ].join(","),
    };
    await mkdir(join(output, "tools"));
    for (const [name, owner] of [
      ["contractgen", "runtime/cmd/contractgen"],
      ["archive", "plugins/scripts/archive"],
    ])
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
          "github.com/Tangerg/flame/" + owner,
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
  offline,
  tools,
}) {
  const rootModule = "github.com/Tangerg/flame";
  const manifest = JSON.parse(
    execFileSync("go", ["mod", "edit", "-json"], { cwd: repository }),
  );
  await writeFile(
    join(output, "go.mod"),
    `module ${artifacts.modulePath}\n\ngo ${manifest.Go}\n\nrequire ${rootModule} ${artifacts.version}\n\ntool (\n${tools.map((name) => "\t" + name).join("\n")}\n)\n`,
  );
  await cp(join(repository, "go.sum"), join(output, "go.sum"));
  const moduleCache = execFileSync("go", ["env", "GOMODCACHE"], {
    cwd: repository,
    encoding: "utf8",
  }).trim();
  const proxy = [
    pathToFileURL(artifacts.proxy).href,
    pathToFileURL(join(moduleCache, "cache/download")).href,
    offline ? "off" : "https://proxy.golang.org",
  ].join(",");
  const env = {
    ...process.env,
    GOWORK: "off",
    GOTOOLCHAIN: "local",
    GONOPROXY: "none",
    GONOSUMDB: rootModule,
    GOPROXY: proxy,
  };
  const run = (args) =>
    execFileSync("go", args, { cwd: output, env, stdio: "inherit" });
  run(["mod", "tidy"]);
  run(["mod", "vendor"]);
  const vendorEnv = {
    ...env,
    GOPROXY: "off",
    GOSUMDB: "off",
    GOFLAGS: "-mod=vendor",
  };
  execFileSync("go", ["generate", "./..."], {
    cwd: output,
    env: vendorEnv,
    stdio: "inherit",
  });
  execFileSync("go", ["test", "./..."], {
    cwd: output,
    env: vendorEnv,
    stdio: "inherit",
  });
  execFileSync("go", ["build", "./..."], {
    cwd: output,
    env: vendorEnv,
    stdio: "inherit",
  });
}
