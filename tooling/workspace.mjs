import { execFileSync } from "node:child_process";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  realpath,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { stageNode } from "./node-companion.mjs";

export const root = fileURLToPath(new URL("../", import.meta.url));
export function run(
  command,
  args,
  cwd = root,
  env = process.env,
  capture = false,
) {
  return execFileSync(command, args, {
    cwd,
    env,
    stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit",
    maxBuffer: 64 * 1024 * 1024,
  });
}
const json = async (path) => JSON.parse(await readFile(path, "utf8"));
export async function workspaces(repository = root) {
  const manifest = await json(join(repository, "package.json"));
  return Promise.all(
    manifest.workspaces.map(async (directory) => ({
      directory: join(repository, directory),
      ...(await json(join(repository, directory, "package.json"))),
    })),
  );
}
function ordered(packages) {
  const byName = new Map(packages.map((p) => [p.name, p])),
    result = [],
    visiting = new Set(),
    seen = new Set();
  const add = (p) => {
    if (seen.has(p.name)) return;
    if (visiting.has(p.name))
      throw new Error(`workspace dependency cycle: ${p.name}`);
    visiting.add(p.name);
    for (const name of new Set([
      ...Object.keys(p.dependencies ?? {}),
      ...Object.keys(p.peerDependencies ?? {}),
      ...(p.flame?.buildDependencies ?? []),
    ])) {
      if (byName.has(name)) add(byName.get(name));
    }
    visiting.delete(p.name);
    seen.add(p.name);
    result.push(p);
  };
  packages.forEach(add);
  return result;
}
export async function buildShared(repository = root) {
  const packages = ordered(await workspaces(repository));
  for (const p of packages)
    if (p.name === "@flame/icons" || p.name === "@flame/ui")
      run("npm", ["run", "build"], p.directory);
}
async function declarationFiles(directory) {
  const found = [];
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    if (entry.name === "node_modules") continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) found.push(...(await declarationFiles(path)));
    else if (entry.name === "declaration.json") found.push(path);
  }
  return found;
}

// The consumer declaration version has one owner, plugin.DeclarationProtocol. Stamp it
// into every first-party package declaration so a bump needs no hand-edited
// manifest; checkGenerated then enforces the projection stays current.
async function stampDeclarations(repository) {
  const method = await readFile(
    join(repository, "packages/go-sdk/plugin/declaration.go"),
    "utf8",
  );
  const owner = method.match(/\bconst DeclarationProtocol = "([^"]+)"/);
  if (!owner)
    throw new Error("plugin.DeclarationProtocol not found in declaration.go");
  const protocol = JSON.stringify(owner[1]);
  for (const base of ["plugins", "examples/plugins"])
    for (const file of await declarationFiles(join(repository, base))) {
      const text = await readFile(file, "utf8");
      const stamped = text.replace(/("protocol":\s*)"[^"]*"/, `$1${protocol}`);
      if (stamped !== text) await writeFile(file, stamped);
    }
}

export async function generate(repository = root) {
  run("node", ["plugins/scripts/generate-runtime-profile.mjs"], repository);
  await buildShared(repository);
  const env = {
    ...proxyEnvironment(join(root, "out/module-proxy")),
    GOWORK: join(repository, "go.work"),
  };
  const go = (args, member = "") =>
    run("go", args, join(repository, member), env);
  const temporary = await mkdtemp(join(tmpdir(), "flame-contract-source-"));
  try {
    go(["generate", "./mcp"], "qimu");
    go(
      ["run", "./cmd/controlgen", "--out", "sdk/src/protocol.generated.ts"],
      "qimu",
    );
    go(
      [
        "run",
        "./examples/document/cmd/webgen",
        "--out",
        "examples/document/web/src/api.generated.ts",
      ],
      "qimu",
    );
    go(
      [
        "run",
        "./examples/authoring/cmd/authorhost",
        "-generate",
        "examples/authoring/browser/src/contracts.generated.ts",
      ],
      "qimu",
    );
    go(
      [
        "run",
        "./cmd/flame-sdk",
        "backend-protocol",
        "--node-release",
        "../local-runtime/node-release.json",
        "--out",
        "../backend-sdk/src/protocol.generated.json",
      ],
      "packages/go-sdk",
    );
    const memory = join(temporary, "memory.json");
    await writeFile(
      memory,
      run(
        "go",
        ["run", "./memory/contractgen"],
        join(repository, "plugins"),
        env,
        true,
      ),
    );
    go(
      [
        "run",
        "github.com/Tangerg/flame/qimu/cmd/qimu",
        "contract",
        "--feature",
        memory,
        "--out",
        "",
        "--go-out",
        "plugin/memory/contract.generated.go",
        "--go-package",
        "memory",
      ],
      "packages/go-sdk",
    );
    run(
      "go",
      [
        "run",
        "./memory/backend/cmd/contracts",
        "--caller-go",
        "../packages/go-sdk/plugin/memory/caller_policy.generated.go",
      ],
      join(repository, "plugins"),
      env,
      true,
    );
    go(["generate", "./..."], "plugins");
    go(
      [
        "run",
        "github.com/Tangerg/flame/qimu/cmd/qimu",
        "contract",
        "--feature",
        "usagereports/contracts/capability.json",
        "--out",
        "usagereports/package/io.github.tangerg.flame/contracts",
        "--ts",
        "usagereports/component",
      ],
      "plugins",
    );
    go(["generate", "./internal/testsupport"], "runtime");
    go(["generate", "./nodequalification", "./integration"], "tooling");
    go(
      [
        "run",
        "./cmd/pluginbrowsergen",
        "--out",
        "../packages/go-sdk/protocol/plugin_browser.generated.go",
      ],
      "runtime",
    );
    go(
      [
        "run",
        "./cmd/contractgen",
        "--out",
        "contract",
        "--validators",
        "../packages/go-sdk/protocol",
        "--ts",
        "../packages/runtime-client",
        "--samples",
        "contract/samples",
        "--go-local",
        ".",
        "--go-client",
        "../packages/go-sdk/client",
      ],
      "runtime",
    );
    go(["generate", "./..."], "apps/cli");
    go(["generate", "./..."], "examples/plugins/reviews/backend");
    go(
      [
        "run",
        "./cmd/bridgegen",
        "--source",
        ".",
        "--receiver",
        "DesktopHost",
        "--out",
        "../web/src/platform/desktop.generated.ts",
        "--foundation",
        "../web/src/foundation/notificationAuthorization.ts",
      ],
      "apps/desktop",
    );
    go(
      [
        "run",
        "./goapi",
        "catalog",
        "--repository",
        repository,
        "--out",
        join(repository, "runtime/contract/go-api.json"),
      ],
      "tooling",
    );
    for (const p of ordered(await workspaces(repository)))
      if (p.scripts?.generate)
        run("npm", ["run", "generate"], p.directory, env);
    for (const destination of [
      "qimu/sdk/testing/generated/plugin-values.json",
      "runtime/testdata/plugin-values.json",
      "packages/runtime-client/testing/generated/plugin-values.json",
    ]) {
      await mkdir(dirname(join(repository, destination)), { recursive: true });
      await cp(
        join(repository, "qimu/jsonvalue/testdata/values.json"),
        join(repository, destination),
      );
    }
    await stampDeclarations(repository);
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
}
export function goWorkspaceMembers(repository = root) {
  const env = { ...process.env, GOWORK: join(repository, "go.work") };
  return JSON.parse(
    run("go", ["work", "edit", "-json"], repository, env, true),
  ).Use.map((entry) => {
    const directory = resolve(repository, entry.DiskPath);
    const manifest = JSON.parse(
      run(
        "go",
        ["mod", "edit", "-json"],
        directory,
        { ...env, GOWORK: "off" },
        true,
      ),
    );
    return { directory, manifest };
  });
}
export async function moduleArtifacts({
  repository = root,
  revision = "HEAD",
  output = join(repository, "out/module-proxy"),
  offline = false,
  targets,
} = {}) {
  const frozen = run(
    "git",
    ["rev-parse", `${revision}^{commit}`],
    repository,
    process.env,
    true,
  )
    .toString()
    .trim();
  const readFrozen = (commit, path) =>
    run("git", ["show", `${commit}:${path}`], repository, process.env, true);
  const bootstrap = await mkdtemp(join(tmpdir(), "flame-proxy-tool-"));
  try {
    let manifestSequence = 0;
    const manifestAt = async (commit, path) => {
      const file = join(bootstrap, `manifest-${manifestSequence++}.mod`);
      await writeFile(file, readFrozen(commit, path));
      return JSON.parse(
        run(
          "go",
          ["mod", "edit", "-json", file],
          bootstrap,
          { ...process.env, GOWORK: "off", GOTOOLCHAIN: "local" },
          true,
        ),
      );
    };
    const frozenWork = join(bootstrap, "source.work");
    await writeFile(frozenWork, readFrozen(frozen, "go.work"));
    const workspace = JSON.parse(
      run(
        "go",
        ["work", "edit", "-json", frozenWork],
        bootstrap,
        process.env,
        true,
      ),
    );
    const members = new Map();
    for (const entry of workspace.Use) {
      const directory = relative(
        repository,
        resolve(repository, entry.DiskPath),
      ).replaceAll("\\", "/");
      if (!directory || directory.startsWith("../"))
        throw new Error(
          "module proxy: members must be repository subdirectories",
        );
      const manifest = await manifestAt(frozen, `${directory}/go.mod`);
      members.set(manifest.Module.Path, { directory, manifest });
    }
    const tooling = await manifestAt(frozen, "tooling/go.mod");
    const modVersion = tooling.Require?.find(
      (dependency) => dependency.Path === "golang.org/x/mod",
    )?.Version;
    if (!modVersion)
      throw new Error(
        "module proxy requires tooling's selected x/mod dependency",
      );
    await writeFile(
      join(bootstrap, "go.mod"),
      `module flame.invalid/module-proxy\n\ngo ${tooling.Go}\n\nrequire golang.org/x/mod ${modVersion}\n`,
    );
    await writeFile(
      join(bootstrap, "go.sum"),
      readFrozen(frozen, "tooling/go.sum"),
    );
    const files = run(
      "git",
      ["ls-tree", "-r", "--name-only", frozen, "--", "tooling/moduleproxy"],
      repository,
      process.env,
      true,
    )
      .toString()
      .trim()
      .split("\n");
    for (const file of files) {
      if (!file) continue;
      const target = join(bootstrap, relative("tooling/moduleproxy", file));
      await mkdir(dirname(target), { recursive: true });
      await writeFile(target, readFrozen(frozen, file));
    }
    const tool = join(bootstrap, "module-proxy");
    const toolEnv = {
      ...process.env,
      GOWORK: "off",
      GOTOOLCHAIN: "local",
      GONOPROXY: "none",
      ...(offline ? { GOPROXY: "off" } : {}),
    };
    run(
      "go",
      ["build", "-buildvcs=false", "-mod=readonly", "-o", tool, "."],
      bootstrap,
      toolEnv,
    );
    const candidates = JSON.parse(readFrozen(frozen, "module-candidates.json"));
    const own = (path) => members.has(path);
    const requested =
      targets ??
      [...members.values()].flatMap(({ manifest }) =>
        (manifest.Require ?? [])
          .filter((dependency) => own(dependency.Path))
          .map((dependency) => ({
            path: dependency.Path,
            version: dependency.Version,
          })),
      );
    const artifacts = {};
    const visiting = new Set();
    const prepare = async ({ path, version }) => {
      const key = `${path}@${version}`;
      if (artifacts[key]) return;
      if (visiting.has(key))
        throw new Error(`module proxy: candidate dependency cycle at ${key}`);
      const member = members.get(path);
      if (!member)
        throw new Error(
          `module proxy: target is not a workspace member: ${path}`,
        );
      const pinned = candidates[path]?.[version];
      if (!pinned)
        throw new Error(
          `module proxy: ${key} has no pinned source in module-candidates.json`,
        );
      const source = run(
        "git",
        ["rev-parse", `${pinned}^{commit}`],
        repository,
        process.env,
        true,
      )
        .toString()
        .trim();
      visiting.add(key);
      const manifest = await manifestAt(source, `${member.directory}/go.mod`);
      if (manifest.Module.Path !== path)
        throw new Error(`module proxy: source module does not match ${key}`);
      for (const dependency of manifest.Require ?? []) {
        if (own(dependency.Path))
          await prepare({ path: dependency.Path, version: dependency.Version });
        else if (dependency.Path === "github.com/Tangerg/flame")
          throw new Error(
            `module proxy: ${key} still requires the retired root module`,
          );
      }
      run(
        tool,
        [
          "--repository",
          repository,
          "--revision",
          source,
          "--output",
          output,
          "--module",
          path,
          "--version",
          version,
          "--subdir",
          member.directory,
        ],
        repository,
        toolEnv,
      );
      artifacts[key] = {
        path,
        version,
        revision: source,
        directory: member.directory,
        go: manifest.Go,
      };
      visiting.delete(key);
    };
    for (const target of requested) await prepare(target);
    return {
      proxy: output,
      revision: frozen,
      modules: artifacts,
      members: Object.fromEntries(
        [...members].map(([path, { directory, manifest }]) => [
          path,
          {
            directory,
            requires: Object.fromEntries(
              (manifest.Require ?? []).map((item) => [item.Path, item.Version]),
            ),
          },
        ]),
      ),
    };
  } finally {
    await rm(bootstrap, { recursive: true, force: true });
  }
}

export function proxyEnvironment(proxy, { offline = false } = {}) {
  return {
    ...process.env,
    GOTOOLCHAIN: "local",
    GONOPROXY: "none",
    GONOSUMDB: "github.com/Tangerg/flame",
    GOPROXY:
      pathToFileURL(proxy).href +
      (offline ? ",off" : ",https://proxy.golang.org"),
  };
}
export async function linkDependencies(repository, packages) {
  packages ??= await workspaces(repository);
  const workspaceNames = new Set(packages.map((pkg) => pkg.name.split("/")[0]));
  const members = [
    "",
    ...packages.map((p) => relative(repository, p.directory)),
  ];
  for (const member of members) {
    const installed = join(root, member, "node_modules");
    if (!existsSync(installed)) continue;
    const destination = join(repository, member, "node_modules");
    await mkdir(destination, { recursive: true });
    for (const entry of await readdir(installed, { withFileTypes: true })) {
      if (workspaceNames.has(entry.name)) continue;
      await symlink(join(installed, entry.name), join(destination, entry.name));
    }
  }
  for (const p of packages) {
    const destination = join(repository, "node_modules", p.name);
    await mkdir(dirname(destination), { recursive: true });
    await symlink(p.directory, destination);
  }
}
async function trackedSources(repository) {
  return run(
    "git",
    ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
    repository,
    process.env,
    true,
  )
    .toString()
    .split("\0")
    .filter((p) => p && existsSync(join(repository, p)));
}
export async function checkGenerated() {
  const scratch = await realpath(
    await mkdtemp(join(tmpdir(), "flame-generated-")),
  );
  try {
    for (const file of await trackedSources(root)) {
      const target = join(scratch, file);
      await mkdir(dirname(target), { recursive: true });
      await cp(join(root, file), target);
    }
    await linkDependencies(scratch);
    run("git", ["init", "-q"], scratch);
    run("git", ["add", "--all"], scratch);
    await generate(scratch);
    const changed = [
      run("git", ["diff", "--name-status"], scratch, process.env, true)
        .toString()
        .trim(),
      run(
        "git",
        ["ls-files", "--others", "--exclude-standard"],
        scratch,
        process.env,
        true,
      )
        .toString()
        .trim(),
    ]
      .filter(Boolean)
      .join("\n");
    if (changed)
      throw new Error(`generated files differ; run make generate:\n${changed}`);
  } finally {
    await rm(scratch, { recursive: true, force: true });
  }
}
async function stageWeb() {
  await buildShared();
  const packages = await workspaces();
  for (const name of ["@flame/plugins", "@flame/web"]) {
    const p = packages.find((p) => p.name === name);
    run("npm", ["run", "build"], p.directory);
  }
  const destination = join(root, "apps/desktop/assets/web");
  await rm(destination, { recursive: true, force: true });
  await cp(join(root, "apps/web/dist"), destination, { recursive: true });
}
async function goMembers(native) {
  const env = {
    ...proxyEnvironment(join(root, "out/module-proxy")),
    GOWORK: "off",
  };
  for (const { directory, manifest } of goWorkspaceMembers()) {
    const requiresNative =
      manifest.Require?.some(
        (p) => p.Path === "github.com/wailsapp/wails/v3",
      ) ?? false;
    if (native && !requiresNative) continue;
    const packages =
      requiresNative && !native
        ? ["./cmd/bridgegen", "./internal/bridge"]
        : ["./..."];
    for (const operation of ["test", "vet", "build"]) {
      const args = [operation, "-mod=readonly"];
      if (operation === "test") args.push("-p=1");
      run("go", [...args, ...packages], directory, env);
    }
    if (native) {
      const output = join(root, "out/native-check");
      await mkdir(output, { recursive: true });
      run(
        "go",
        ["build", "-mod=readonly", "-o", join(output, "flame"), "."],
        directory,
        env,
      );
    }
  }
}
async function checkQimuAuthors() {
  const temporary = await mkdtemp(join(tmpdir(), "qimu-authors-"));
  const binary = join(
    temporary,
    process.platform === "win32" ? "authorhost.exe" : "authorhost",
  );
  const env = {
    ...proxyEnvironment(join(root, "out/module-proxy")),
    GOWORK: "off",
    QIMU_AUTHOR_HOST_BINARY: binary,
  };
  try {
    run(
      "go",
      [
        "build",
        "-mod=readonly",
        "-o",
        binary,
        "./examples/authoring/cmd/authorhost",
      ],
      join(root, "qimu"),
      env,
    );
    run(
      "npm",
      ["run", "test:workflow"],
      join(root, "qimu/examples/authoring/browser"),
      env,
    );
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
}
async function main(command) {
  switch (command) {
    case "setup": {
      const expected = JSON.parse(
        run("go", ["work", "edit", "-json"], root, process.env, true),
      ).Go;
      const actual = run("go", ["env", "GOVERSION"], root, process.env, true)
        .toString()
        .trim();
      if (actual !== `go${expected}`)
        throw new Error(`Go ${expected} is required; found ${actual}`);
      const artifacts = await moduleArtifacts();
      const env = proxyEnvironment(artifacts.proxy);
      run("npm", ["ci"], root);
      for (const { directory } of goWorkspaceMembers())
        run("go", ["mod", "download"], directory, { ...env, GOWORK: "off" });
      await buildShared();
      const target = JSON.parse(
        run("go", ["env", "-json", "GOOS", "GOARCH"], root, process.env, true),
      );
      await stageNode({
        os: target.GOOS,
        arch: target.GOARCH,
        output: join(root, "out/toolchains"),
      });
      return;
    }
    case "generate":
      return generate();
    case "check-generated":
      return checkGenerated();
    case "check":
      await buildShared();
      run("npm", ["run", "test:tooling"]);
      run("node", ["tooling/check-boundaries.mjs"]);
      run("npm", ["run", "lint"]);
      run("npm", ["run", "format:check"]);
      await checkGenerated();
      await goMembers(false);
      for (const p of ordered(await workspaces())) {
        const script = p.scripts?.check
          ? "check"
          : p.scripts?.typecheck
            ? "typecheck"
            : null;
        if (script) run("npm", ["run", script], p.directory);
      }
      await checkQimuAuthors();
      run("node", ["tooling/qualify-node.mjs"]);
      run("git", ["diff", "--check"]);
      return;
    case "check-native":
      if (process.platform !== "darwin")
        throw new Error(
          "native verification requires the supported macOS host",
        );
      await stageWeb();
      await goMembers(true);
      run("wails3", ["task", "build"], join(root, "apps/desktop"));
      return;
    case "check-author":
      return run("node", ["tooling/check-author.mjs"]);
    case "stage-web":
      return stageWeb();
    case "dev-web":
      await buildShared();
      return run("npm", ["run", "dev"], join(root, "apps/web"));
    case "dev-desktop":
      await stageWeb();
      return run("wails3", ["task", "dev"], join(root, "apps/desktop"));
    case "package-desktop":
      await stageWeb();
      return run("wails3", ["task", "package"], join(root, "apps/desktop"));
    case "build-runtime":
    case "build-cli": {
      const target = JSON.parse(
        run("go", ["env", "-json", "GOOS", "GOARCH"], root, process.env, true),
      );
      const kind = command === "build-runtime" ? "runtime" : "cli",
        directory = join(root, "out", kind, `${target.GOOS}-${target.GOARCH}`);
      await mkdir(directory, { recursive: true });
      await stageNode({
        os: target.GOOS,
        arch: target.GOARCH,
        output: directory,
        offline: true,
      });
      run("node", [
        "plugins/scripts/build-packages.mjs",
        "--os",
        target.GOOS,
        "--arch",
        target.GOARCH,
        "--output",
        join(directory, "plugins"),
        "--replace",
      ]);
      run(
        "go",
        [
          "build",
          "-trimpath",
          "-o",
          join(directory, "qimu-runner"),
          "./runner/cmd/qimu-runner",
        ],
        join(root, "qimu"),
        { ...proxyEnvironment(join(root, "out/module-proxy")), GOWORK: "off" },
      );
      return run(
        "go",
        [
          "build",
          "-trimpath",
          "-o",
          join(directory, kind === "runtime" ? "flame-runtime" : "flame"),
          kind === "runtime" ? "./cmd/flame-runtime" : ".",
        ],
        join(root, kind === "runtime" ? "runtime" : "apps/cli"),
        { ...proxyEnvironment(join(root, "out/module-proxy")), GOWORK: "off" },
      );
    }
    default:
      throw new Error(`unknown workspace command: ${command}`);
  }
}
if (
  process.argv[1] &&
  resolve(process.argv[1]) === fileURLToPath(import.meta.url)
)
  await main(process.argv[2]);
