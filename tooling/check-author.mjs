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
import { run, root, buildShared, proxyEnvironment } from "./workspace.mjs";
const scratch = await mkdtemp(join(tmpdir(), "flame-author-check-"));
const loopbackProbe = `
import net from "node:net";
import { readFileSync } from "node:fs";
const server = net.createServer(socket => socket.end("callback"));
await new Promise((resolve, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolve);
});
const client = net.connect({host: "127.0.0.1", port: server.address().port});
let body = "";
client.on("data", chunk => body += chunk);
await new Promise((resolve, reject) => {
  client.once("end", resolve);
  client.once("error", reject);
});
await new Promise(resolve => server.close(resolve));
if (body !== "callback") throw new Error("loopback callback was lost");
const external = net.connect({host: "203.0.113.1", port: 443});
external.setTimeout(1000, () => external.destroy(new Error("external access was not denied")));
const refusal = await new Promise(resolve => external.once("error", resolve));
if (!["EPERM", "EACCES"].includes(refusal.code)) throw refusal;
let sourceDenied = false;
try { readFileSync(process.argv[1]); }
catch (error) { sourceDenied = ["EPERM", "EACCES"].includes(error.code); }
if (!sourceDenied) throw new Error("checkout source access was not denied");
`;
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
  for (const name of ["memory", "guardrails", "usagereports", "deliverables"]) {
    const managed = ["usagereports", "deliverables"].includes(name);
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
    // SDK callback fixtures need local HTTP; generation and builds remain fully offline.
    const callbackPolicy = `${policy}
      (allow network-bind (local ip "localhost:*"))
      (allow network-inbound (local ip "localhost:*"))
      (allow network-outbound (remote ip "localhost:*"))`;
    if (!managed) {
      isolated(
        callbackPolicy,
        [
          "node",
          "--input-type=module",
          "-e",
          loopbackProbe,
          join(root, "AGENTS.md"),
        ],
        project,
        env,
      );
      for (const command of ["test", "generate", "build"])
        isolated(
          command === "test" ? callbackPolicy : policy,
          ["go", command, "./..."],
          project,
          env,
        );
    }
    if (managed) {
      if (existsSync(join(project, "go.mod")))
        throw new Error(
          "managed author project retains a Go compiler prerequisite",
        );
      const noGo = join(scratch, name + "-no-go");
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
      for (const command of npmCommands) {
        // macOS refuses a different nested Seatbelt profile. This suite proves
        // its own per-invocation isolation and inherited-profile refusal.
        if (
          name === "deliverables" &&
          process.platform === "darwin" &&
          command[0] === "test"
        )
          run("npm", command, project, env);
        else
          isolated(
            command[0] === "test" ? callbackPolicy : policy,
            ["npm", ...command],
            project,
            env,
          );
      }
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

run(
  "go",
  ["test", "-count=1", "-tags", "author", "./integration"],
  join(root, "tooling"),
  { ...proxyEnvironment(join(root, "out/module-proxy")), GOWORK: "off" },
);
