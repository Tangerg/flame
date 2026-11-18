import { existsSync } from "node:fs";
import { join, relative } from "node:path";
import { run, root, goWorkspaceMembers } from "./workspace.mjs";

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
for (const { directory, manifest } of members) {
  if (manifest.Replace?.length)
    throw new Error(`${directory} uses a local dependency replacement`);
}
console.log(
  "Workspace manifests enumerate every independently buildable Go module.",
);
