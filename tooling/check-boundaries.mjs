import { existsSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { run, root, workspaces, goWorkspaceMembers } from "./workspace.mjs";
import { assertSourceClosure, readSourceGraph } from "./source-graph.mjs";

const members = goWorkspaceMembers();
if (existsSync(join(root, "go.mod")) || existsSync(join(root, "go.sum")))
  throw new Error(
    "the repository root coordinates workspaces and must not own a Go module",
  );
const registered = new Set(
  members.map(({ directory }) => relative(root, directory) + "/go.mod"),
);
const manifests = run(
  "git",
  ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
  root,
  process.env,
  true,
)
  .toString()
  .split("\0")
  .filter((path) => path.endsWith("/go.mod") && existsSync(join(root, path)));
if (
  registered.size !== members.length ||
  new Set(manifests).size !== registered.size ||
  manifests.some((path) => !registered.has(path))
)
  throw new Error(
    "go.work must enumerate every maintained Go module exactly once",
  );
const paths = new Set();
for (const { directory, manifest } of members) {
  if (manifest.Replace?.length)
    throw new Error(`${directory} uses a local dependency replacement`);
  if (paths.has(manifest.Module.Path))
    throw new Error(`duplicate module identity: ${manifest.Module.Path}`);
  paths.add(manifest.Module.Path);
  if (manifest.Module.Path === "github.com/Tangerg/flame")
    throw new Error("the retired root module is still a member");
  for (const dependency of manifest.Require ?? [])
    if (dependency.Path === "github.com/Tangerg/flame")
      throw new Error(`${directory} requires the retired root module`);
    else if (
      manifest.Module.Path !== "github.com/Tangerg/flame/tooling" &&
      dependency.Path === "github.com/Tangerg/flame/tooling"
    )
      throw new Error(`${directory} requires repository tooling`);
}
const dependencies = (member, tests = false) =>
  run(
    "go",
    [
      "list",
      "-deps",
      ...(tests ? ["-test"] : []),
      "-f",
      "{{.ImportPath}}\t{{with .Module}}{{.Path}}{{end}}",
      "./...",
    ],
    join(root, member),
    { ...process.env, GOWORK: join(root, "go.work") },
    true,
  )
    .toString()
    .trim()
    .split("\n")
    .map((line) => {
      const [path, module] = line.split("\t");
      return { path, module };
    });
const sdk = "github.com/Tangerg/flame/packages/go-sdk";
const qimu = "github.com/Tangerg/flame/qimu";
for (const { path, module } of dependencies("packages/go-sdk", true))
  if (
    module.startsWith("github.com/Tangerg/flame/") &&
    module !== sdk &&
    module !== qimu
  )
    throw new Error(
      `SDK source, tests or tools compile a host module: ${path}`,
    );
for (const { path, module } of dependencies("qimu", true))
  if (module.startsWith("github.com/Tangerg/flame/") && module !== qimu)
    throw new Error(
      `Qimu source, tests or tools compile a consumer module: ${path}`,
    );
for (const { path: dependency } of dependencies("runtime"))
  if (dependency.startsWith("github.com/Tangerg/flame/plugins/"))
    throw new Error(
      `Runtime production imports a downstream plugin: ${dependency}`,
    );
for (const member of ["runtime", "apps/cli", "plugins"])
  for (const { path: dependency } of dependencies(member))
    if (dependency.startsWith("github.com/Tangerg/flame/tooling/"))
      throw new Error(`${member} imports repository tooling: ${dependency}`);

const packages = await workspaces();
const runtimeClient = packages.find(
  (pkg) => pkg.name === "@flame/runtime-client",
);
const qimuSDK = packages.find((pkg) => pkg.name === "@qimu/sdk");
const jsonEntry = resolve(qimuSDK.directory, qimuSDK.exports["./json"]);
const runtimeEntries = new Set(
  Object.entries(runtimeClient.exports)
    .filter(([name, path]) => name !== "./json" && !path.includes("*"))
    .map(([, path]) => resolve(runtimeClient.directory, path)),
);
assertSourceClosure(qimuSDK.directory, jsonEntry, runtimeEntries);
const byName = new Map(packages.map((pkg) => [pkg.name, pkg]));
for (const pkg of packages.filter((pkg) => pkg.name.startsWith("@qimu/"))) {
  const visited = new Set();
  const inspect = (current) => {
    if (visited.has(current.name)) return;
    visited.add(current.name);
    for (const field of ["dependencies", "devDependencies", "peerDependencies"])
      for (const name of Object.keys(current[field] ?? {})) {
        if (name.startsWith("@flame/"))
          throw new Error(`${pkg.name} compiles a consumer package: ${name}`);
        if (byName.has(name)) inspect(byName.get(name));
      }
  };
  inspect(pkg);
}
const { graph } = readSourceGraph(join(root, "apps/web"));
for (const [file, edges] of Object.entries(graph))
  if (file.startsWith("extensions/"))
    for (const dependency of edges)
      if (dependency.startsWith("features/") || dependency.startsWith("app/"))
        throw new Error(
          `generic extensions depend on product policy: ${file} -> ${dependency}`,
        );
for (const pkg of packages)
  for (const field of ["dependencies", "devDependencies", "peerDependencies"])
    for (const name of Object.keys(pkg[field] ?? {}))
      if (["@flame/runtime-contract", "@flame/plugin-sdk"].includes(name))
        throw new Error(`${pkg.name} retains obsolete package ${name}`);
console.log(
  "Workspace manifests, SDK source/test/tool closures, Runtime direction and Web mechanisms passed.",
);
