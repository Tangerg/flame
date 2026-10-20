import { execFileSync } from "node:child_process";
import { cp, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
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
