import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "vite";

assert.equal(process.platform, "darwin");
assert.ok(process.argv[2], "run through the desktop task graph");
const fixturePath = process.env.FLAME_NATIVE_RUNTIME_FIXTURE;
assert.ok(fixturePath, "set FLAME_NATIVE_RUNTIME_FIXTURE to an explicit private fixture JSON path");
const fixtureFile = await stat(fixturePath);
assert.equal(fixtureFile.mode & 0o077, 0, "the fixture contains a token and must be private");
const fixture = JSON.parse(await readFile(fixturePath, "utf8"));
await assert.rejects(stat(fixture.workspacePath), { code: "ENOENT" });
const execute = promisify(execFile);
const directory = await mkdtemp(join(tmpdir(), "flame-native-runtime-"));
try {
  const assets = join(directory, "assets");
  const frontend = fileURLToPath(new URL("..", import.meta.url));
  await build({
    configFile: join(frontend, "vite.config.ts"),
    root: frontend,
    publicDir: false,
    build: {
      outDir: assets,
      emptyOutDir: true,
      rollupOptions: {
        input: join(frontend, "visual/plugin-runtime-native.ts"),
        output: { entryFileNames: "probe.js" },
      },
    },
  });
  await writeFile(
    join(assets, "index.html"),
    '<!doctype html><div id="page" style="height:600px"></div><script type="module" src="/probe.js"></script>',
  );
  const executable = join(directory, "probe");
  await execute(
    "go",
    [
      "build",
      "-tags",
      "production,pluginruntimeprobe",
      "-ldflags",
      process.argv[2],
      "-o",
      executable,
      ".",
    ],
    {
      cwd: fileURLToPath(new URL("../..", import.meta.url)),
      timeout: 120000,
      maxBuffer: 1024 * 1024,
    },
  );
  const { stdout } = await execute(executable, [assets, fixturePath], {
    timeout: 40000,
    maxBuffer: 1024 * 1024,
  });
  const results = stdout.split("\n").filter((value) => value.startsWith("{"));
  assert.equal(results.length, 1, "the native gate must publish exactly one terminal result");
  const result = JSON.parse(results[0]);
  console.log(result);
  assert.equal(result.error, undefined);
  assert.deepEqual(result, { Calls: 1, Ready: 2, Connected: 2 });
} finally {
  await rm(directory, { recursive: true, force: true });
}
