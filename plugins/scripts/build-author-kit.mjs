import {
  access,
  cp,
  mkdir,
  readFile,
  readdir,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { isAbsolute, join, relative, sep } from "node:path";
import { parseArgs } from "node:util";
import { createConsumerLock } from "../../tooling/npm-consumer-lock.mjs";
import {
  moduleArtifacts,
  root,
  run,
  linkDependencies,
} from "../../tooling/workspace.mjs";
import {
  prepareGoKit,
  prepareDevelopmentTools,
  authorSDK,
} from "./build-go-kit.mjs";
import { validateDestination, readBuildPlan } from "./build.mjs";
import { authorDependencies } from "./author-dependencies.mjs";
import { stageNode } from "../../tooling/node-companion.mjs";

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
if (!plugin || !/^[a-z][a-z0-9-]*$/.test(plugin))
  throw new Error("a source plugin name is required");
const source = join(root, "plugins", plugin);
const plan = await readBuildPlan(source);
const managed = plan.managed.length > 0;
if (
  !output ||
  !isAbsolute(output) ||
  !plugin ||
  !/^[a-z][a-z0-9-]*$/.test(plugin) ||
  (plan.native &&
    (!modulePath ||
      modulePath === "github.com/Tangerg/flame" ||
      modulePath.startsWith("github.com/Tangerg/flame/") ||
      !/^[-a-zA-Z0-9.]+\.[-a-zA-Z0-9.]+\/[a-zA-Z0-9._~/-]+$/.test(modulePath) ||
      modulePath
        .split("/")
        .some((part) => part === "." || part === ".." || part === ""))) ||
  (!plan.native && modulePath !== undefined)
)
  throw new Error(
    "--plugin and an absolute new --output are required; native backends also require --module-path in the author's namespace",
  );
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
const inputs = [
  `plugins/${plugin}`,
  "plugins/scripts",
  "plugins/tsconfig.json",
  "plugins/backend.tsconfig.json",
  "plugins/package.json",
  "tooling",
  "packages/icons",
  "packages/ui",
  "packages/runtime-client",
  "packages/component-sdk",
  "packages/backend-sdk",
  "packages/local-runtime/node-release.json",
  "package.json",
  "package-lock.json",
  "go.work",
  "module-candidates.json",
  "packages/go-sdk",
  "plugins/go.mod",
  "plugins/go.sum",
  "examples/plugins/reviews/backend/go.mod",
];
try {
  run(
    "git",
    ["diff", "--exit-code", "--quiet", revision, "--", ...inputs],
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
    ["ls-files", "--others", "--exclude-standard", "--", ...inputs],
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
const sdk = authorSDK(artifacts);
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
      ["backend", "internal", "component", "contracts", "package"].some(
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
  const tools = new Set([
    "github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk",
  ]);
  async function rewrite(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) await rewrite(path);
      else if (entry.name.endsWith(".go")) {
        let text = await readFile(path, "utf8");
        text = text.replaceAll(
          `github.com/Tangerg/flame/plugins/${plugin}/`,
          `${modulePath}/`,
        );
        text = text.replaceAll(
          /go run (github\.com\/Tangerg\/flame\/packages\/go-sdk\/cmd\/[a-zA-Z0-9_-]+)/g,
          (_, tool) => {
            tools.add(tool);
            return `go tool ${tool}`;
          },
        );
        if (/go run github\.com\/Tangerg\/flame\//.test(text))
          throw new Error(
            `author source retains an unsupported Flame command: ${path}`,
          );
        await writeFile(path, text);
      }
    }
  }
  if (plan.native && backend) {
    await rewrite(join(output, "backend"));
    if (await exists(join(output, "internal")))
      await rewrite(join(output, "internal"));
  }
  if (plan.native)
    await prepareGoKit({
      repository: root,
      output,
      artifacts,
      offline,
      modulePath,
      tools: [...tools],
    });
  if (managed) {
    await prepareDevelopmentTools({
      repository: root,
      output,
      artifacts,
      offline,
    });
    await cp(
      new URL("./author-package.mjs", import.meta.url),
      join(output, "package.mjs"),
    );
    await cp(
      new URL("./author-generate.mjs", import.meta.url),
      join(output, "generate.mjs"),
    );
  }
  const component = await exists(join(output, "component/component.tsx"));
  if (component || managed) {
    const packageDirectories = [
      "packages/runtime-client",
      ...(component
        ? ["packages/icons", "packages/ui", "packages/component-sdk"]
        : []),
      ...(managed ? ["packages/backend-sdk"] : []),
    ];
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
        "packages/runtime-client",
        "packages/component-sdk",
        "packages/backend-sdk",
        "package.json",
        "package-lock.json",
        "plugins/package.json",
        "plugins/tsconfig.json",
        "plugins/backend.tsconfig.json",
      ],
      root,
      process.env,
      true,
    );
    await mkdir(frozenTree);
    run("tar", ["-xf", frozenTar, "-C", frozenTree], root, process.env, true);
    await rm(frozenTar);
    const candidates = await Promise.all(
      packageDirectories.map(async (directory) => ({
        directory: join(frozenTree, directory),
        manifest: JSON.parse(
          await readFile(join(frozenTree, directory, "package.json"), "utf8"),
        ),
      })),
    );
    await linkDependencies(
      frozenTree,
      candidates.map(({ directory, manifest }) => ({ directory, ...manifest })),
    );
    const lock = JSON.parse(
      await readFile(join(frozenTree, "package-lock.json"), "utf8"),
    );
    const { dependencies, devDependencies, packages } =
      await authorDependencies({
        source: output,
        manifest: JSON.parse(
          await readFile(join(frozenTree, "plugins/package.json"), "utf8"),
        ),
        lock,
        packages: candidates,
        required: [
          ...(component ? ["@flame/component-sdk"] : []),
          ...(managed ? ["@flame/backend-sdk"] : []),
        ],
      });
    const packedDependencies = [];
    for (const {
      directory: packageDirectory,
      manifest,
      development,
    } of packages) {
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
      const resolved = `file:./artifacts/npm/${packed.filename}`;
      (development ? devDependencies : dependencies)[packed.name] = resolved;
      packedDependencies.push({
        manifest,
        resolved,
        integrity: packed.integrity,
      });
    }
    const version = (name) => {
      const entry = lock.packages[`node_modules/${name}`];
      if (!entry?.version) throw new Error(`root lock has no selected ${name}`);
      return entry.version;
    };
    for (const name of [
      "typescript",
      "@types/react",
      "@types/react-dom",
      "@types/node",
      "rolldown",
      ...(managed ? ["vitest"] : []),
    ])
      devDependencies[name] = version(name);
    const consumer = {
      name: `${plugin}-author`,
      private: true,
      type: "module",
      scripts: {
        typecheck:
          managed && component
            ? "tsc --noEmit && tsc -p backend.tsconfig.json"
            : "tsc --noEmit",
        ...(component ? { build: "node build-component.mjs" } : {}),
        ...(managed
          ? {
              generate: "node generate.mjs",
              package: "node package.mjs",
              test: "node test-backend.mjs",
            }
          : {}),
      },
      dependencies,
      devDependencies,
    };
    await writeFile(
      join(output, "package.json"),
      JSON.stringify(consumer, null, 2) + "\n",
    );
    await writeFile(
      join(output, "package-lock.json"),
      JSON.stringify(
        createConsumerLock(lock, consumer, packedDependencies),
        null,
        2,
      ) + "\n",
    );
    for (const [source, destination, include] of [
      [
        component ? "tsconfig.json" : "backend.tsconfig.json",
        "tsconfig.json",
        component ? "component" : "backend",
      ],
      ...(managed && component
        ? [["backend.tsconfig.json", "backend.tsconfig.json", "backend"]]
        : []),
    ]) {
      const config = JSON.parse(
        await readFile(join(frozenTree, "plugins", source), "utf8"),
      );
      config.include = [include];
      await writeFile(
        join(output, destination),
        JSON.stringify(config, null, 2) + "\n",
      );
    }
    // Every packing input has been consumed; drop the frozen tree before the
    // author project's own install so it never ships inside the release.
    await rm(frozenTree, { recursive: true, force: true });
    if (component)
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
    if (component) run("npm", ["run", "build"], output);
    if (managed) {
      const node = await stageNode({
        os: process.platform,
        arch: process.arch === "x64" ? "amd64" : process.arch,
        output: join(output, "tools"),
        offline,
      });
      await writeFile(
        join(output, "test-backend.mjs"),
        `import { execFileSync } from "node:child_process";\nimport { fileURLToPath } from "node:url";\nexecFileSync(fileURLToPath(new URL(${JSON.stringify(relative(output, node).split(sep).join("/"))}, import.meta.url)), [fileURLToPath(new URL("./node_modules/vitest/vitest.mjs", import.meta.url)), "run", "backend", "contracts"], { cwd: fileURLToPath(new URL("./", import.meta.url)), stdio: "inherit" });\n`,
      );
      run("npm", ["run", "generate"], output);
      run("npm", ["test"], output);
    }
  }
  await writeFile(
    join(output, "README.md"),
    managed && !plan.native
      ? `# ${plugin} TypeScript author project\n\nImplementation source is frozen at ${revision}. Packed public SDKs and this project's lock own the npm graph. There is no Go source or Go compiler prerequisite. The prebuilt flame-sdk contract and archive commands are development-only tools; the included selected Node companion runs backend and contract tests.\n\nFrom this directory:\n\n\`\`\`sh\nnpm ci --ignore-scripts\nnpm run generate\nnpm run typecheck\nnpm test\nnpm run build\nnpm run package -- --output /absolute/new/release --archives /absolute/new/archives\n\`\`\`\n\nThe portable ZIP contains its complete backend graph and declared resources, without node_modules or an interpreter. Install it through Runtime; Runtime supplies its qualified Node companion. Generation projects only the feature contracts named by the package declaration; a Tool-only package has none. Rebuild and explicitly select a new release after edits. Preserve prior installations and their original pending work when changing releases.\n`
      : `# ${plugin} author project\n\nThe backend, component and package resource root use the same layout as Flame.\nYour module is ${modulePath}; its implementation source is frozen at ${revision}. Flame dependencies are immutable module artifacts ${sdk.path}@${sdk.version} from ${sdk.revision}.\nGo production and generation tools are vendored; there is no local replace or Flame checkout dependency.\nJS dependencies use packed SDKs and this project's own lock. They require an ordinary npm install and are not claimed to be fully offline.\n\nRun with the Go version in go.mod:\n\n\`\`\`sh\nGOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go test ./...\nGOWORK=off GOTOOLCHAIN=local GOPROXY=off GONOPROXY=none GOFLAGS=-mod=vendor go generate ./...\n${component ? "npm ci --ignore-scripts\nnpm run typecheck\nnpm run build\n" : ""}node build.mjs --source "$PWD" --output /absolute/new/release --os darwin --arch arm64\n\`\`\`\n\nInstall the built release; installation needs neither Go nor Node. Existing consumer contracts come from the pinned public SDK; local Go types implement them without redefining their identity. Regenerate package/UI projections after edits. New business contracts must be defined and published explicitly by their owner.\n`,
  );
  console.log(output);
} catch (error) {
  await rm(output, { recursive: true, force: true });
  throw error;
}
