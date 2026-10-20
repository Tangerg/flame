import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
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
import { isAbsolute, join, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { prepareGoKit } from "./build-go-kit.mjs";

const { values } = parseArgs({
  options: {
    output: { type: "string" },
    plugin: { type: "string", default: "usage" },
    offline: { type: "boolean", default: false },
  },
});
const { output, plugin, offline } = values;
if (!output || !isAbsolute(output) || !/^[a-z][a-z0-9-]*$/.test(plugin))
  throw new Error(
    "usage: node plugins/build-author-kit.mjs --plugin <directory> --output /absolute/new/directory [--offline]",
  );
const repository = fileURLToPath(new URL("../", import.meta.url));
const source = join(repository, "plugins", plugin);
await access(join(source, "plugin.json"));
async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}
const backend = await exists(join(source, "backend"));
const component = await exists(join(source, "src", "component.tsx"));
const run = (command, args, cwd, env = process.env) =>
  execFileSync(command, args, {
    cwd,
    env,
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 32 * 1024 * 1024,
  });
const writeJSON = (path, value) =>
  writeFile(join(output, path), JSON.stringify(value, null, 2) + "\n");

// Exclusive creation bounds cleanup to the export owned by this invocation.
await mkdir(output);
try {
  await cp(source, output, {
    recursive: true,
    filter: (path) =>
      !["node_modules", "vendor", "bin", "build", ".git"].includes(
        path.split(/[\\/]/).at(-1),
      ),
  });
  await cp(join(repository, "plugins", "build.mjs"), join(output, "build.mjs"));
  await cp(
    join(repository, "plugins", "cmd", "archive"),
    join(output, "cmd", "archive"),
    { recursive: true },
  );
  if (backend) {
    await cp(join(repository, "plugins", "go.mod"), join(output, "go.mod"));
    await cp(join(repository, "plugins", "go.sum"), join(output, "go.sum"));
    run(
      "go",
      ["mod", "edit", "-module", `github.com/Tangerg/flame/plugins/${plugin}`],
      output,
    );
    await prepareGoKit({ repository, output, offline });
    console.log(`${plugin}: independent Go module and vendored public SDK`);
  } else
    await writeFile(
      join(output, "go.mod"),
      `module github.com/Tangerg/flame/plugins/${plugin}\n\ngo 1.27.0\n`,
    );
  const scripts = {};
  if (component) {
    for (const directory of ["icons", "ui"])
      run("npm", ["run", "build"], join(repository, directory));
    const artifacts = join(output, "artifacts", "npm");
    await mkdir(artifacts, { recursive: true });
    const dependencies = {};
    for (const directory of [
      "icons",
      "ui",
      "runtime/contract/typescript",
      "plugin-sdk",
    ]) {
      const [packed] = JSON.parse(
        run(
          "npm",
          [
            "pack",
            "--ignore-scripts",
            "--json",
            "--pack-destination",
            artifacts,
          ],
          join(repository, directory),
        ),
      );
      dependencies[packed.name] = `file:./artifacts/npm/${packed.filename}`;
    }
    const lock = JSON.parse(
      await readFile(join(repository, "plugins", "package-lock.json")),
    );
    const version = (name) => lock.packages[`node_modules/${name}`].version;
    for (const name of ["react", "react-dom", "marked"])
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
    scripts.typecheck = "tsc --noEmit";
    scripts.build = "node build-component.mjs";
    await writeJSON("package.json", {
      name: `flame-${plugin}-author`,
      private: true,
      type: "module",
      scripts,
      dependencies,
      devDependencies,
    });
    const tsconfig = JSON.parse(
      await readFile(join(repository, "plugins", "tsconfig.json")),
    );
    await writeJSON("tsconfig.json", { ...tsconfig, include: ["src"] });
    await writeFile(
      join(output, "build-component.mjs"),
      `import { rolldown } from "rolldown";
import { buildComponent } from "@flame/plugin-sdk/build";
import { fileURLToPath } from "node:url";
await buildComponent({ compiler: rolldown,
 input: fileURLToPath(new URL("./src/component.tsx", import.meta.url)),
 directory: fileURLToPath(new URL("./io.github.tangerg.flame/components", import.meta.url)),
});
`,
    );
    const npmOptions = ["--ignore-scripts", ...(offline ? ["--offline"] : [])];
    run("npm", ["install", "--package-lock-only", ...npmOptions], output);
    run("npm", ["ci", ...npmOptions], output);
    const authorRoot = await realpath(output);
    for (const name of Object.keys(dependencies))
      assert.ok(
        (await realpath(join(output, "node_modules", name))).startsWith(
          `${authorRoot}${sep}`,
        ),
        `author dependency escapes its artifact directory: ${name}`,
      );
    run("npm", ["run", "typecheck"], output);
    run("npm", ["run", "build"], output);
    const resources = "io.github.tangerg.flame/components";
    const files = (await readdir(join(source, resources))).sort();
    assert.deepEqual((await readdir(join(output, resources))).sort(), files);
    for (const file of files)
      assert.deepEqual(
        await readFile(join(output, resources, file)),
        await readFile(join(source, resources, file)),
        "rebuild the bundled component before exporting its author kit",
      );
    console.log(
      `${plugin}: independent UI build reproduces the packaged component`,
    );
  }
  await writeFile(
    join(output, "AUTHORING.md"),
    `# Independent ${plugin} author project

This project contains the complete plugin source, declaration, contracts and build tools.
${
  backend
    ? `Its Go module imports only public Flame APIs. The included SDK source archives are
content-addressed development artifacts, not versions published to a registry.
Dependencies are vendored; no Flame checkout or local replace is required.
`
    : "Its UI dependencies include local package archives; no Flame checkout is required.\n"
}

Run with the Go version declared in go.mod and Node.js:

\`\`\`sh
GOWORK=off GOPROXY=off go test ./...
GOWORK=off go generate ./...
${component ? "npm ci --ignore-scripts\nnpm run typecheck\nnpm run build\n" : ""}node build.mjs --os darwin --arch arm64 --output /absolute/new/packages --archives /absolute/new/releases
\`\`\`

Choose the Runtime host OS and architecture when building a native backend.
The resulting ZIP contains ${backend ? "the prebuilt backend and" : "only"} declared resources;
install that ZIP with the ordinary Runtime plugins.install operation or CLI.
Users installing the release need no Go or Node toolchain. Native confinement
availability still follows the actual host platform.

${
  backend
    ? `After changing Go dependencies, run node vendor-go.mjs to refresh vendor from the
included SDK archives and ordinary released dependencies. Plugin generation writes
only this project's own contracts; host projections belong to their consumers.
`
    : ""
}
`,
  );
  console.log(output);
} catch (error) {
  await rm(output, { recursive: true, force: true });
  if (error.stdout) process.stderr.write(error.stdout);
  if (error.stderr) process.stderr.write(error.stderr);
  throw error;
}
