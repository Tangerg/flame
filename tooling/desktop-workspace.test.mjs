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

async function runDesktopCommand(t, command) {
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
  await mkdir(join(directory, "apps/desktop"), { recursive: true });
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
        scripts: { build: "unused" },
      }),
    );
  }
  const bin = join(directory, "bin");
  await mkdir(bin);
  const log = join(directory, "commands.jsonl");
  for (const executable of ["npm", "wails3"]) {
    await writeFile(
      join(bin, executable),
      `#!${process.execPath}\nimport { appendFileSync } from "node:fs";\nappendFileSync(process.env.FLAME_DESKTOP_TEST_LOG, JSON.stringify([${JSON.stringify(executable)}, ...process.argv.slice(2)]) + "\\n");\n`,
      { mode: 0o755 },
    );
  }
  const result = spawnSync(
    process.execPath,
    [join(directory, "tooling/workspace.mjs"), command],
    {
      cwd: directory,
      env: {
        ...process.env,
        PATH: `${bin}:${process.env.PATH}`,
        FLAME_DESKTOP_TEST_LOG: log,
      },
      encoding: "utf8",
    },
  );
  let commands = [];
  try {
    commands = (await readFile(log, "utf8"))
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line));
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  return { result, commands };
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
