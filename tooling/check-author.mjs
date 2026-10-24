import {
  mkdtemp,
  mkdir,
  readdir,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { existsSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { run, root, buildShared } from "./workspace.mjs";
const scratch = await mkdtemp(join(tmpdir(), "flame-author-check-"));
function isolated(policy, command, directory, env, expectedError) {
  // Pipes keep the sandbox from inheriting a repository-local redirected log
  // descriptor. Its source-read denial also applies to descriptor metadata.
  const result = spawnSync(
    "/usr/bin/sandbox-exec",
    ["-p", policy, ...command],
    { cwd: directory, env, encoding: "utf8", maxBuffer: 64 * 1024 * 1024 },
  );
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.error) throw result.error;
  if (expectedError) {
    if (
      result.status === null ||
      result.status === 0 ||
      !expectedError.test(result.stdout + result.stderr)
    )
      throw new Error(
        `isolated author command did not reject the invalid source: ${command.join(" ")}`,
      );
    return;
  }
  if (result.status !== 0)
    throw new Error(
      `isolated author command failed (${result.signal ?? result.status}): ${command.join(" ")}`,
    );
}
async function verifyTypecheckEnvironments(policy, project, env) {
  const browser = join(project, "component/authoring-environment.probe.ts");
  const backend = join(project, "backend/authoring-environment.probe.ts");
  try {
    await writeFile(
      browser,
      'export {};\ndeclare global { interface AuthoringEnvironmentProbe { runtime: "browser"; } }\n',
    );
    await writeFile(
      backend,
      'export {};\ndeclare global { interface AuthoringEnvironmentProbe { runtime: "node"; } }\n',
    );
    isolated(policy, ["npm", "run", "typecheck"], project, env);
    await writeFile(
      backend,
      "const values: string[] = [];\nexport const required: string = values[0];\n",
    );
    isolated(policy, ["npm", "run", "typecheck"], project, env, /TS2322/);
  } finally {
    await rm(browser, { force: true });
    await rm(backend, { force: true });
  }
}
async function snapshot(directory) {
  const entries = [];
  for (const item of await readdir(directory, { withFileTypes: true })) {
    if (["node_modules", "vendor", "artifacts"].includes(item.name)) continue;
    const path = join(directory, item.name);
    if (item.isDirectory())
      for (const [file, value] of await snapshot(path))
        entries.push([item.name + "/" + file, value]);
    else entries.push([item.name, await readFile(path)]);
  }
  return entries;
}
try {
  await buildShared();
  for (const name of ["memory", "guardrails", "usagereports"]) {
    const managed = name === "usagereports";
    const project = join(
      scratch,
      name === "memory" ? "memory author 作者" : name,
    );
    run("node", [
      "plugins/scripts/build-author-kit.mjs",
      "--plugin",
      name,
      ...(!managed
        ? ["--module-path", `example.com/flame-author/${name}`]
        : []),
      "--output",
      project,
      "--offline",
    ]);
    const before = new Map(await snapshot(project));
    const cache = join(scratch, name + "-cache"),
      modules = join(scratch, name + "-modules");
    await mkdir(cache);
    await mkdir(modules);
    const env = {
      ...process.env,
      GOWORK: "off",
      GOTOOLCHAIN: "local",
      GOPROXY: "off",
      GONOPROXY: "none",
      GOSUMDB: "off",
      GOFLAGS: "-mod=vendor",
      GOMODCACHE: modules,
      GOCACHE: cache,
    };
    if (!existsSync("/usr/bin/sandbox-exec"))
      throw new Error(
        "offline author verification requires the supported macOS network sandbox",
      );
    const policy = `(version 1) (allow default) (deny network*) (deny file-read* (subpath ${JSON.stringify(root)}))`;
    if (!managed)
      for (const command of ["test", "generate", "build"])
        isolated(policy, ["go", command, "./..."], project, env);
    if (managed) {
      if (existsSync(join(project, "go.mod")))
        throw new Error(
          "managed author project retains a Go compiler prerequisite",
        );
      const noGo = join(scratch, "no-go");
      await mkdir(noGo);
      await writeFile(
        join(noGo, "go"),
        '#!/bin/sh\necho "Go is unavailable in the TypeScript author environment" >&2\nexit 97\n',
        { mode: 0o755 },
      );
      env.PATH = noGo + ":" + process.env.PATH;
      await verifyTypecheckEnvironments(policy, project, env);
    }
    const npmCommands = managed
      ? [
          ["ci", "--ignore-scripts", "--offline"],
          ["run", "generate"],
          ["run", "typecheck"],
          ["test"],
          ["run", "build"],
          [
            "run",
            "package",
            "--",
            "--output",
            join(scratch, name + "-release"),
            "--archives",
            join(scratch, name + "-archives"),
          ],
        ]
      : [
          ["ci", "--ignore-scripts", "--offline"],
          ["run", "typecheck"],
          ["run", "build"],
        ];
    if (existsSync(join(project, "package.json")))
      for (const command of npmCommands)
        isolated(policy, ["npm", ...command], project, env);
    const after = new Map(await snapshot(project));
    if (
      before.size !== after.size ||
      [...before].some(([file, value]) => !after.get(file)?.equals(value))
    )
      throw new Error(
        `offline generation changed ${name}'s authored or generated bytes`,
      );
  }
} finally {
  await rm(scratch, { recursive: true, force: true });
}

run("go", ["test", "-count=1", "-tags", "author", "./tests/integration"]);
