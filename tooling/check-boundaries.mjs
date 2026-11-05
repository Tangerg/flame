import { existsSync } from "node:fs";
import { join, relative } from "node:path";
import { run, root, workspaces, goWorkspaceMembers } from "./workspace.mjs";
import { readSourceGraph } from "./source-graph.mjs";

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
    if (dependency.Path === "github.com/Tangerg/qimu")
      throw new Error(`${directory} requires the retired Qimu module`);
    else if (dependency.Path === "github.com/Tangerg/flame")
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
for (const { path, module } of dependencies("packages/go-sdk", true))
  if (module.startsWith("github.com/Tangerg/flame/") && module !== sdk)
    throw new Error(
      `SDK source, tests or tools compile a host module: ${path}`,
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
      if (
        name.startsWith("@qimu/") ||
        ["@flame/runtime-contract", "@flame/plugin-sdk"].includes(name)
      )
        throw new Error(`${pkg.name} retains obsolete package ${name}`);
console.log(
  "Workspace manifests, SDK source/test/tool closures, Runtime direction and Web mechanisms passed.",
);
