import { execFileSync } from "node:child_process";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

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

// The backend protocol version has one owner, capability.Protocol. Stamp it
// into every first-party package declaration so a bump needs no hand-edited
// manifest; checkGenerated then enforces the projection stays current.
async function stampDeclarations(repository) {
  const method = await readFile(
    join(repository, "runtime/capability/method.go"),
    "utf8",
  );
  const owner = method.match(/\bconst Protocol = "([^"]+)"/);
  if (!owner) throw new Error("capability.Protocol not found in method.go");
  const protocol = JSON.stringify(owner[1]);
  for (const base of ["plugins", "examples/plugins"])
    for (const file of await declarationFiles(join(repository, base))) {
      const text = await readFile(file, "utf8");
      const stamped = text.replace(/("protocol":\s*)"[^"]*"/, `$1${protocol}`);
      if (stamped !== text) await writeFile(file, stamped);
    }
}

export async function generate(repository = root) {
  await buildShared(repository);
  run("go", ["generate", "./plugins/..."], repository);
  const memory = join(
    repository,
    "runtime/internal/adapter/integration/memorycontract",
  );
  run(
    "go",
    [
      "run",
      "./plugins/memory/backend/cmd/contracts",
      "--caller-go",
      join(memory, "caller_policy.generated.go"),
    ],
    repository,
  );
  run("go", ["generate", "./runtime/...", "./apps/cli/..."], repository);
  const members = JSON.parse(
    run("go", ["work", "edit", "-json"], repository, process.env, true),
  ).Use;
  for (const member of members) {
    const directory = resolve(repository, member.DiskPath);
    if (directory === repository) continue;
    const manifest = JSON.parse(
      run("go", ["mod", "edit", "-json"], directory, process.env, true),
    );
    if (manifest.Tool?.length) run("go", ["generate", "./..."], directory);
  }
  for (const p of ordered(await workspaces(repository)))
    if (p.scripts?.generate) run("npm", ["run", "generate"], p.directory);
  await stampDeclarations(repository);
}
export async function moduleArtifacts({
  repository = root,
  revision = "HEAD",
  output = join(root, "out/module-proxy"),
  offline = false,
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
  const bootstrap = await mkdtemp(join(tmpdir(), "flame-proxy-tool-"));
  try {
    const frozenManifest = async (file) => {
      const path = join(bootstrap, file.replaceAll("/", "-") + ".mod");
      await writeFile(
        path,
        run(
          "git",
          ["show", `${frozen}:${file}`],
          repository,
          process.env,
          true,
        ),
      );
      return JSON.parse(
        run(
          "go",
          ["mod", "edit", "-json", path],
          bootstrap,
          { ...process.env, GOWORK: "off", GOTOOLCHAIN: "local" },
          true,
        ),
      );
    };
    const manifest = await frozenManifest("go.mod");
    const modVersion = manifest.Require.find(
      (p) => p.Path === "golang.org/x/mod",
    )?.Version;
    if (!modVersion)
      throw new Error(
        "module proxy requires the root's selected x/mod dependency",
      );
    await writeFile(
      join(bootstrap, "go.mod"),
      `module flame.invalid/module-proxy\n\ngo ${manifest.Go}\n\nrequire golang.org/x/mod ${modVersion}\n`,
    );
    await writeFile(
      join(bootstrap, "go.sum"),
      run("git", ["show", `${frozen}:go.sum`], repository, process.env, true),
    );
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
      [
        "build",
        "-mod=mod",
        "-o",
        tool,
        join(repository, "tooling/moduleproxy/main.go"),
      ],
      bootstrap,
      toolEnv,
    );
    const local = manifest.Require.find(
      (p) => p.Path === "github.com/Tangerg/flame/packages/local-runtime",
    );
    const author = await frozenManifest(
      "examples/plugins/reviews/backend/go.mod",
    );
    const product = author.Require.find(
      (p) => p.Path === "github.com/Tangerg/flame",
    );
    for (const [selected, subdir] of [
      [local, "packages/local-runtime"],
      [product, ""],
    ]) {
      if (!selected)
        throw new Error(
          "candidate dependencies must be selected before freezing source",
        );
      run(
        tool,
        [
          "--repository",
          repository,
          "--revision",
          frozen,
          "--output",
          output,
          "--module",
          selected.Path,
          "--version",
          selected.Version,
          "--subdir",
          subdir,
        ],
        repository,
        toolEnv,
      );
    }
    return {
      proxy: output,
      version: product.Version,
      localVersion: local.Version,
      revision: frozen,
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
async function linkDependencies(repository) {
  const members = [
    "",
    ...(await workspaces()).map((p) => relative(root, p.directory)),
  ];
  for (const member of members) {
    const installed = join(root, member, "node_modules");
    if (!existsSync(installed)) continue;
    const destination = join(repository, member, "node_modules");
    await mkdir(destination, { recursive: true });
    for (const entry of await readdir(installed, { withFileTypes: true })) {
      if (entry.name === "@flame") continue;
      await symlink(join(installed, entry.name), join(destination, entry.name));
    }
  }
  const names = join(repository, "node_modules/@flame");
  await mkdir(names, { recursive: true });
  for (const p of await workspaces(repository))
    if (p.name.startsWith("@flame/"))
      await symlink(p.directory, join(repository, "node_modules", p.name));
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
  const scratch = await mkdtemp(join(tmpdir(), "flame-generated-"));
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
  const entries = JSON.parse(
    run("go", ["work", "edit", "-json"], root, process.env, true),
  ).Use;
  for (const entry of entries) {
    const directory = resolve(root, entry.DiskPath);
    const manifest = JSON.parse(
      run("go", ["mod", "edit", "-json"], directory, process.env, true),
    );
    const requiresNative =
      manifest.Require?.some(
        (p) => p.Path === "github.com/wailsapp/wails/v3",
      ) ?? false;
    if (requiresNative !== native) continue;
    for (const operation of ["test", "vet", "build"]) {
      const args = [operation, "-mod=readonly"];
      // Process-backed fixtures own their compiler and backend concurrency.
      if (operation === "test") args.push("-p=1");
      if (native && operation === "build") {
        const output = join(root, "out/native-check");
        await mkdir(output, { recursive: true });
        args.push("-o", join(output, "flame"));
      }
      args.push("./...");
      run("go", args, directory, env);
    }
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
      run("go", ["mod", "download"], root, env);
      await buildShared();
      return;
    }
    case "generate":
      return generate();
    case "check-generated":
      return checkGenerated();
    case "check":
      await buildShared();
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
      return run("go", [
        "build",
        "-trimpath",
        "-o",
        join(directory, kind === "runtime" ? "flame-runtime" : "flame"),
        kind === "runtime" ? "./runtime/cmd/flame-runtime" : "./apps/cli",
      ]);
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
