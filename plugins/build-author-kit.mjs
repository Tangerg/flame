import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  cp,
  mkdir,
  readFile,
  readdir,
  realpath,
  rm,
  writeFile,
} from "node:fs/promises";
import { isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";

const args = process.argv.slice(2);
const outputIndex = args.indexOf("--output");
const output = args[outputIndex + 1];
if (
  outputIndex !== 0 ||
  !output ||
  !isAbsolute(output) ||
  args.some((arg, index) => index > 1 && arg !== "--offline")
) {
  throw new Error(
    "usage: node plugins/build-author-kit.mjs --output /absolute/new/directory [--offline]",
  );
}

const root = fileURLToPath(new URL("../", import.meta.url));
const run = (command, parameters, cwd) =>
  execFileSync(command, parameters, { cwd, stdio: "pipe" });
const writeJSON = (name, value) =>
  writeFile(join(output, name), `${JSON.stringify(value, null, 2)}\n`);
const lock = JSON.parse(
  await readFile(join(root, "plugins/package-lock.json")),
);
const version = (name) => lock.packages[`node_modules/${name}`].version;

// Exclusive creation makes cleanup safe even when the caller mistypes a path.
await mkdir(output);
try {
  for (const directory of ["icons", "ui"])
    run("npm", ["run", "build"], join(root, directory));
  const artifacts = join(output, "artifacts");
  await mkdir(artifacts);
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
        ["pack", "--ignore-scripts", "--json", "--pack-destination", artifacts],
        join(root, directory),
      ),
    );
    dependencies[packed.name] = `file:./artifacts/${packed.filename}`;
  }
  for (const name of ["react", "react-dom"]) dependencies[name] = version(name);
  const devDependencies = {};
  for (const name of [
    "typescript",
    "@types/react",
    "@types/react-dom",
    "@types/node",
    "rolldown",
  ])
    devDependencies[name] = version(name);
  await writeJSON("package.json", {
    name: "flame-usage-author",
    private: true,
    type: "module",
    scripts: { typecheck: "tsc --noEmit", build: "node build.mjs" },
    dependencies,
    devDependencies,
  });
  const tsconfig = JSON.parse(
    await readFile(join(root, "plugins/tsconfig.json")),
  );
  await writeJSON("tsconfig.json", { ...tsconfig, include: ["usage/src"] });
  await cp(join(root, "plugins/usage"), join(output, "usage"), {
    recursive: true,
    filter: (path) => !path.endsWith("/components"),
  });
  await writeFile(
    join(output, "build.mjs"),
    `import { rolldown } from "rolldown";
import { buildComponent } from "@flame/plugin-sdk/build";
import { fileURLToPath } from "node:url";
await buildComponent({
  compiler: rolldown,
  input: fileURLToPath(new URL("./usage/src/component.tsx", import.meta.url)),
  directory: fileURLToPath(new URL("./usage/io.github.tangerg.flame/components", import.meta.url)),
});
`,
  );
  await writeFile(
    join(output, "README.md"),
    `# Standalone Usage author kit

This complete bundled feature uses only the included public package artifacts
and declared npm dependencies. Copy this directory away from the Flame checkout,
then run \`npm ci --ignore-scripts\`, \`npm run typecheck\` and \`npm run build\`.
Install the resulting \`usage\` directory through the ordinary plugin installer.
No backend, host source, private import or StyleX compiler is required.

The tarballs are current-source artifacts, not a claim of registry publication.
Qualification builds the component without loading or rendering its UI.
`,
  );
  run(
    "npm",
    [
      "install",
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      ...(args.includes("--offline") ? ["--offline"] : []),
    ],
    output,
  );
  const authorRoot = await realpath(output);
  for (const name of Object.keys(dependencies))
    assert.ok(
      (await realpath(join(output, "node_modules", name))).startsWith(
        `${authorRoot}/`,
      ),
      `author dependency escapes its artifact directory: ${name}`,
    );
  run("npm", ["run", "typecheck"], output);
  run("npm", ["run", "build"], output);
  const component = "usage/io.github.tangerg.flame/components";
  const expected = join(root, "plugins", component);
  const actual = join(output, component);
  const files = (await readdir(expected)).sort();
  assert.deepEqual((await readdir(actual)).sort(), files);
  for (const file of files)
    assert.deepEqual(
      await readFile(join(actual, file)),
      await readFile(join(expected, file)),
      `standalone Usage artifact differs: ${file}`,
    );
  console.log(`Standalone Usage author kit: ${output}`);
} catch (error) {
  await rm(output, { recursive: true, force: true });
  if (error.stdout) process.stderr.write(error.stdout);
  if (error.stderr) process.stderr.write(error.stderr);
  throw new Error(error.message);
}
