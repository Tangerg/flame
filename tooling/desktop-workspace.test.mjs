import { strict as assert } from "node:assert";
import { spawnSync } from "node:child_process";
import {
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { root } from "./workspace.mjs";
import { desktopProduct } from "../apps/desktop/build/product.mjs";

async function runDesktopCommand(
  t,
  command,
  minimumVersion = desktopProduct.macOS.minimumVersion,
) {
  const directory = await mkdtemp(join(tmpdir(), "flame-desktop-workspace-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  await mkdir(join(directory, "tooling"));
  for (const file of [
    "workspace.mjs",
    "node-companion.mjs",
    "presentation.mjs",
  ]) {
    await cp(join(root, "tooling", file), join(directory, "tooling", file));
  }
  await symlink(join(root, "node_modules"), join(directory, "node_modules"));
  await mkdir(join(directory, "packages/local-runtime"), { recursive: true });
  await cp(
    join(root, "packages/local-runtime/node-release.json"),
    join(directory, "packages/local-runtime/node-release.json"),
  );
  await mkdir(join(directory, "apps/desktop/build"), { recursive: true });
  await cp(
    join(root, "apps/desktop/build/product.mjs"),
    join(directory, "apps/desktop/build/product.mjs"),
  );
  const workspaces = ["packages/icons", "packages/ui"];
  await writeFile(
    join(directory, "package.json"),
    JSON.stringify({ type: "module", workspaces }),
  );
  for (const [index, member] of workspaces.entries()) {
    await mkdir(join(directory, member), { recursive: true });
    await writeFile(
      join(directory, member, "package.json"),
      JSON.stringify({
        name: index === 0 ? "@flame/icons" : "@flame/ui",
        exports: { ".": "./dist/index.js" },
        scripts: { build: "unused" },
      }),
    );
  }
  const bin = join(directory, "bin");
  await mkdir(bin);
  const log = join(directory, "commands.jsonl");
  for (const executable of ["npm", "wails3", "go", "gofmt", "xcrun"]) {
    await writeFile(
      join(bin, executable),
      `#!${process.execPath}
import { appendFileSync } from "node:fs";
const command = ${JSON.stringify(executable)};
const args = process.argv.slice(2);
appendFileSync(process.env.FLAME_DESKTOP_TEST_LOG, JSON.stringify({command, args, environment: {
  CGO_ENABLED: process.env.CGO_ENABLED,
  CGO_CFLAGS: process.env.CGO_CFLAGS,
  CGO_LDFLAGS: process.env.CGO_LDFLAGS,
  MACOSX_DEPLOYMENT_TARGET: process.env.MACOSX_DEPLOYMENT_TARGET,
}}) + "\\n");
if (command === "go" && args[0] === "work") console.log(JSON.stringify({Use: [{DiskPath: "./apps/desktop"}]}));
if (command === "go" && args[0] === "mod") console.log(JSON.stringify({Require: [{Path: "github.com/wailsapp/wails/v3"}]}));
if (command === "xcrun") console.log("platform MACOS\\nminos " + process.env.FLAME_DESKTOP_TEST_MINIMUM);
`,
      { mode: 0o755 },
    );
  }
  const entry = join(directory, "workspace.mjs");
  await symlink(join(directory, "tooling/workspace.mjs"), entry);
  const result = spawnSync(process.execPath, [entry, command], {
    cwd: directory,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      FLAME_DESKTOP_TEST_LOG: log,
      FLAME_DESKTOP_TEST_MINIMUM: minimumVersion,
    },
    encoding: "utf8",
  });
  let calls = [];
  try {
    calls = (await readFile(log, "utf8"))
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line));
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  return {
    result,
    calls,
    commands: calls.map(({ command, args }) => [command, ...args]),
  };
}

test("Desktop packaging gives the Task graph sole frontend build ownership", async (t) => {
  const { result, commands } = await runDesktopCommand(t, "package-desktop");
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(commands, [["wails3", "task", "package"]]);
});

test("Desktop development prepares shared assets and lets Vite own the frontend", async (t) => {
  const { result, commands } = await runDesktopCommand(t, "dev-desktop");
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(commands, [
    ["npm", "run", "build"],
    ["npm", "run", "build"],
    ["wails3", "task", "dev"],
  ]);
});

test(
  "native Go checks use the product deployment policy for every compiled artifact",
  { skip: process.platform !== "darwin" },
  async (t) => {
    const { result, calls } = await runDesktopCommand(t, "check-native");
    assert.equal(result.status, 0, result.stderr);
    const builds = calls.filter(
      ({ command, args }) =>
        command === "go" && ["test", "vet", "build"].includes(args[0]),
    );
    assert.ok(builds.length > 0);
    for (const { args, environment } of builds) {
      assert.equal(
        args[args.indexOf("-ldflags") + 1],
        desktopProduct.macOS.linkerFlags,
      );
      assert.deepEqual(environment, {
        CGO_ENABLED: "1",
        CGO_CFLAGS: desktopProduct.macOS.compilerFlags,
        CGO_LDFLAGS: desktopProduct.macOS.compilerFlags,
        MACOSX_DEPLOYMENT_TARGET: desktopProduct.macOS.minimumVersion,
      });
    }
  },
);

test(
  "native qualification rejects a binary with a different deployment target",
  { skip: process.platform !== "darwin" },
  async (t) => {
    const { result } = await runDesktopCommand(t, "check-native", "11.0");
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /native verification: expected macOS/);
    assert.match(result.stderr, /found 11\.0/);
  },
);

test(
  "native qualification rejects unsupported hosts before executing build tools",
  { skip: process.platform === "darwin" },
  async (t) => {
    const { result, commands } = await runDesktopCommand(t, "check-native");
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      /native verification requires the supported macOS host/,
    );
    assert.deepEqual(commands, []);
  },
);
