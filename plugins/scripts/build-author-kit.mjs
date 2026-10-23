import {
  access,
  cp,
  mkdir,
  readFile,
  readdir,
  realpath,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { isAbsolute, join, sep } from "node:path";
import { parseArgs } from "node:util";
import { moduleArtifacts, root, run } from "../../tooling/workspace.mjs";
import { prepareGoKit } from "./build-go-kit.mjs";
import { validateDestination } from "./build.mjs";

const { values } = parseArgs({
  options: {
    output: { type: "string" },
    plugin: { type: "string" },
    "module-path": { type: "string" },
    revision: { type: "string", default: "HEAD" },
    offline: { type: "boolean", default: false },
  },
});
const { output, plugin, offline } = values,
  modulePath = values["module-path"];
if (
  !output ||
  !isAbsolute(output) ||
  !plugin ||
  !/^[a-z][a-z0-9-]*$/.test(plugin) ||
  !modulePath ||
  modulePath === "github.com/Tangerg/flame" ||
  modulePath.startsWith("github.com/Tangerg/flame/") ||
  !/^[-a-zA-Z0-9.]+\.[-a-zA-Z0-9.]+\/[a-zA-Z0-9._~/-]+$/.test(modulePath) ||
  modulePath
    .split("/")
    .some((part) => part === "." || part === ".." || part === "")
)
  throw new Error(
    "--plugin, --module-path in the author's namespace, and an absolute new --output are required",
  );
const source = join(root, "plugins", plugin);
await validateDestination(source, output);
await access(join(source, "package/plugin.json"));
const revision = run(
  "git",
  ["rev-parse", `${values.revision}^{commit}`],
  root,
  process.env,
  true,
)
  .toString()
  .trim();
try {
  run(
    "git",
    ["diff", "--exit-code", "--quiet", revision, "--"],
    root,
    process.env,
    true,
  );
} catch {
  throw new Error(
    "author export requires source frozen in the selected commit",
  );
}
if (
  run(
    "git",
    ["ls-files", "--others", "--exclude-standard"],
    root,
    process.env,
    true,
  )
    .toString()
    .trim()
)
  throw new Error(
    "author export requires all maintained source to be frozen before packing",
  );

const artifacts = await moduleArtifacts({
  revision,
  output: join(root, "out/module-proxy"),
  offline,
});
artifacts.modulePath = modulePath;
const exists = async (path) => {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
};
await mkdir(output);
try {
  // Materialize the plugin source from the frozen commit, not the live working
  // tree, so an ignored or untracked file cannot enter the author project.
  const tarball = join(output, ".source.tar");
  const authorPaths = run(
    "git",
    ["ls-tree", "--name-only", revision, `plugins/${plugin}/`],
    root,
    process.env,
    true,
  )
    .toString()
    .trim()
    .split("\n")
    .filter((path) =>
      ["backend", "component", "package", "README.md"].some(
        (entry) => path === `plugins/${plugin}/${entry}`,
      ),
    );
  run(
    "git",
    [
      "archive",
      "--format=tar",
      `--output=${tarball}`,
      revision,
      "--",
      ...authorPaths,
    ],
    root,
    process.env,
    true,
  );
  run(
    "tar",
    ["-xf", tarball, "-C", output, "--strip-components=2"],
    root,
    process.env,
    true,
  );
  await rm(tarball);
  await cp(new URL("./build.mjs", import.meta.url), join(output, "build.mjs"));
  const backend = await exists(join(output, "backend"));
  const tools = new Set(["github.com/Tangerg/flame/plugins/scripts/archive"]);
  async function rewrite(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) await rewrite(path);
      else if (entry.name.endsWith(".go")) {
        let text = await readFile(path, "utf8");
        text = text.replaceAll(
          `github.com/Tangerg/flame/plugins/${plugin}/backend`,
          `${modulePath}/backend`,
        );
        text = text.replaceAll(
          /go run (github\.com\/Tangerg\/flame\/runtime\/cmd\/[a-zA-Z0-9_-]+)/g,
          (_, tool) => {
            tools.add(tool);
            return `go tool ${tool}`;
          },
        );
        await writeFile(path, text);
      }
    }
  }
  if (backend) await rewrite(join(output, "backend"));
  await prepareGoKit({
    repository: root,
    output,
    artifacts,
    offline,
    tools: [...tools],
  });
  const component = await exists(join(output, "component/component.tsx"));
  if (component) {
    const npmArtifacts = join(output, "artifacts/npm");
    await mkdir(npmArtifacts, { recursive: true });
    // Materialize every packing input from the frozen commit into one tree, then
    // build and pack inside it. A git-ignored or stale working-tree file is
    // invisible to the diff/untracked gates above, so packing from the live tree
    // could leak it into a published SDK; packing from this tree cannot.
    // node_modules is the shared toolchain, symlinked rather than archived.
    const frozenTree = join(output, ".frozen");
    const frozenTar = join(output, ".frozen.tar");
    run(
      "git",
      [
        "archive",
        "--format=tar",
        `--output=${frozenTar}`,
        revision,
        "--",
        "packages/icons",
        "packages/ui",
        "packages/runtime-client/typescript",
        "packages/component-sdk",
        "package.json",
        "package-lock.json",
        "plugins/tsconfig.json",
      ],
      root,
      process.env,
      true,
    );
    await mkdir(frozenTree);
    run("tar", ["-xf", frozenTar, "-C", frozenTree], root, process.env, true);
    await rm(frozenTar);
    await symlink(join(root, "node_modules"), join(frozenTree, "node_modules"));
    const dependencies = {};
    for (const directory of [
      "packages/icons",
      "packages/ui",
      "packages/runtime-client/typescript",
      "packages/component-sdk",
    ]) {
      const packageDirectory = join(frozenTree, directory);
      const manifest = JSON.parse(
        await readFile(join(packageDirectory, "package.json"), "utf8"),
      );
      if (manifest.scripts?.build)
        run("npm", ["run", "build"], packageDirectory, process.env, true);
      const [packed] = JSON.parse(
        run(
          "npm",
          [
            "pack",
            "--ignore-scripts",
            "--json",
            "--pack-destination",
            npmArtifacts,
          ],
          packageDirectory,
          process.env,
          true,
        ),
      );
      if (packed.files.some((file) => /\.test\.[cm]?[jt]sx?$/.test(file.path)))
        throw new Error(`${packed.name} ships ordinary test source`);
      dependencies[packed.name] = `file:./artifacts/npm/${packed.filename}`;
    }
    const lock = JSON.parse(
      await readFile(join(frozenTree, "package-lock.json"), "utf8"),
    );
    const version = (name) => {
      const entry = lock.packages[`node_modules/${name}`];
      if (!entry?.version) throw new Error(`root lock has no selected ${name}`);
      return entry.version;
    };
    for (const name of ["react", "react-dom", "motion", "marked"])
      dependencies[name] = version(name);
    const devDependencies = {};
    for (const name of [
      "typescript",
      "@types/react",
      "@types/react-dom",
      "@types/node",
      "rolldown",
    ])
      devDependencies[name] = version(name);
    await writeFile(
      join(output, "package.json"),
      JSON.stringify(
        {
          name: `${plugin}-author`,
          private: true,
          type: "module",
          scripts: {
            typecheck: "tsc --noEmit",
            build: "node build-component.mjs",
          },
          dependencies,
          devDependencies,
        },
        null,
        2,
      ) + "\n",
    );
    const config = JSON.parse(
      await readFile(join(frozenTree, "plugins/tsconfig.json"), "utf8"),
    );
    config.include = ["component"];
    await writeFile(
      join(output, "tsconfig.json"),
      JSON.stringify(config, null, 2) + "\n",
    );
    // Every packing input has been consumed; drop the frozen tree before the
    // author project's own install so it never ships inside the release.
    await rm(frozenTree, { recursive: true, force: true });
    await cp(
      new URL("./author-component.mjs", import.meta.url),
      join(output, "build-component.mjs"),
    );
    run(
      "npm",
      ["install", "--ignore-scripts", ...(offline ? ["--offline"] : [])],
      output,
    );
    const author = await realpath(output);
    for (const name of Object.keys(dependencies))
      if (
        !(await realpath(join(output, "node_modules", name))).startsWith(
          author + sep,
        )
      )
        throw new Error(`author dependency escapes its own project: ${name}`);
    run("npm", ["run", "typecheck"], output);
    run("npm", ["run", "build"], output);
  }
  await writeFile(
    join(output, "AUTHORING.md"),
    `# ${plugin} author project\n\nThe backend, component and package resource root use the same layout as Flame.\nYour module is ${modulePath}; its implementation source is frozen at ${revision}. Flame dependencies are immutable module artifacts ${artifacts.version} from ${artifacts.revision}.\nGo production and generation tools are vendored; there is no local replace or Flame checkout dependency.\nJS dependencies use packed SDKs and this project's own lock. They require an ordinary npm install and are not claimed to be fully offline.\n\nRun with the Go version in go.mod:\n\n\`\`\`sh\nGOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go test ./...\nGOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go generate ./...\n${component ? "npm ci --ignore-scripts\nnpm run typecheck\nnpm run build\n" : ""}node build.mjs --source "$PWD" --output /absolute/new/release --os darwin --arch arm64\n\`\`\`\n\nInstall the built release; installation needs neither Go nor Node. Existing consumer contracts come from the pinned public SDK; local Go types implement them without redefining their identity. Regenerate package/UI projections after edits. New business contracts must be defined and published explicitly by their owner.\n`,
  );
  console.log(output);
} catch (error) {
  await rm(output, { recursive: true, force: true });
  throw error;
}
