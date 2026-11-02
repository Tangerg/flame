import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmod,
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
import { pathToFileURL } from "node:url";

test("parallel Node staging publishes only a verified executable", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "flame-node-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const executable = "bin/node";
  const payload = Buffer.from("node fixture");
  const source = join(root, "source", "node", executable);
  await mkdir(join(root, "source/node/bin"), { recursive: true });
  await writeFile(source, payload, { mode: 0o755 });
  const archive = "node.tar.gz";
  const tar = join(root, archive);
  execFileSync("tar", ["czf", tar, "-C", join(root, "source"), "node"]);
  const bytes = await readFile(tar);
  const sha256 = (value) => createHash("sha256").update(value).digest("hex");
  const target = {
    archive,
    archiveSHA256: sha256(bytes),
    executable,
    executableSHA256: sha256(payload),
  };
  await mkdir(join(root, "packages/local-runtime"), { recursive: true });
  await writeFile(
    join(root, "packages/local-runtime/node-release.json"),
    JSON.stringify({
      version: "test",
      targets: { "linux-amd64": target },
    }),
  );
  await mkdir(join(root, "tooling"));
  const module = join(root, "tooling/node-companion.mjs");
  await cp(new URL("./node-companion.mjs", import.meta.url), module);
  const { stageNode } = await import(pathToFileURL(module));
  const options = {
    os: "linux",
    arch: "amd64",
    output: join(root, "out"),
    cache: join(root, "cache"),
  };
  await mkdir(options.cache);
  await writeFile(join(options.cache, archive), bytes.subarray(0, 3));
  await assert.rejects(stageNode({ ...options, offline: true }), /checksum/);
  t.mock.method(globalThis, "fetch", async () => new Response(bytes));
  const [a, b] = await Promise.all([stageNode(options), stageNode(options)]);
  assert.equal(a, b);
  assert.deepEqual(await readFile(a), payload);
  await chmod(a, 0o644);
  await assert.rejects(stageNode(options), /regular executable/);
  await rm(a);
  await symlink(source, a);
  await assert.rejects(stageNode(options), /redirected/);
});
