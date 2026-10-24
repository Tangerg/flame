import { run, root, workspaces } from "./workspace.mjs";
import { join, resolve } from "node:path";
import {
  assertSourceClosure,
  readSourceGraph,
} from "../apps/web/scripts/source-graph.mjs";

const runtimeClient = (await workspaces()).find(
  (pkg) => pkg.name === "@flame/runtime-client",
);
const jsonEntry = resolve(
  runtimeClient.directory,
  runtimeClient.exports["./json"],
);
const runtimeEntries = new Set(
  Object.entries(runtimeClient.exports)
    .filter(([name, path]) => name !== "./json" && !path.includes("*"))
    .map(([, path]) => resolve(runtimeClient.directory, path)),
);
assertSourceClosure(runtimeClient.directory, jsonEntry, runtimeEntries);
const dependencies = (...targets) =>
  run(
    "go",
    ["list", "-deps", "-f", "{{.ImportPath}}", ...targets],
    root,
    process.env,
    true,
  )
    .toString()
    .trim()
    .split("\n");
for (const target of [
  "./packages/runtime-client/go",
  "./packages/plugin-sdk/...",
]) {
  for (const dependency of dependencies(target))
    if (
      dependency === "github.com/Tangerg/flame/runtime" ||
      /^github\.com\/Tangerg\/flame\/runtime\/internal\/(?:domain|application|adapter|infra|delivery|bootstrap)(?:\/|$)/.test(
        dependency,
      )
    )
      throw new Error(
        `${target} compiles Runtime host implementation: ${dependency}`,
      );
}
for (const target of ["./runtime/...", "./runtime/internal/testsupport"]) {
  for (const dependency of dependencies(target))
    if (dependency.startsWith("github.com/Tangerg/flame/plugins/"))
      throw new Error(`Runtime imports a downstream plugin: ${dependency}`);
}
const selected = run(
  "go",
  ["list", "-m", "-f", "{{.Path}}", "all"],
  root,
  process.env,
  true,
)
  .toString()
  .trim()
  .split("\n");
for (const retired of [
  "github.com/Tangerg/flame/runtime",
  "github.com/Tangerg/flame/cli",
  "github.com/Tangerg/flame/plugins",
  "github.com/Tangerg/flame/desktop",
  "github.com/Tangerg/flame/runtime/localruntime",
])
  if (selected.includes(retired))
    throw new Error(`retired module still selected: ${retired}`);
const { graph } = readSourceGraph(join(root, "apps/web"));
for (const [file, edges] of Object.entries(graph))
  if (file.startsWith("extensions/"))
    for (const dependency of edges)
      if (dependency.startsWith("features/") || dependency.startsWith("app/"))
        throw new Error(
          `generic extensions depend on product policy: ${file} -> ${dependency}`,
        );
for (const pkg of await workspaces())
  for (const field of ["dependencies", "devDependencies", "peerDependencies"])
    for (const name of Object.keys(pkg[field] ?? {}))
      if (["@flame/runtime-contract", "@flame/plugin-sdk"].includes(name))
        throw new Error(`${pkg.name} retains obsolete package ${name}`);
console.log(
  "SDK production closures, Runtime direction and Web mechanism boundaries passed.",
);
