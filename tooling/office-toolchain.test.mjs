import { strict as assert } from "node:assert";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmod,
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";
import { test } from "node:test";

async function fixture(t) {
  const temporary = await mkdtemp(join(tmpdir(), "flame-office-toolchain-"));
  t.after(() => rm(temporary, { recursive: true, force: true }));
  const write = async (path, bytes) => {
    await mkdir(dirname(path), { recursive: true });
    await writeFile(path, bytes);
  };
  const hash = (bytes, algorithm = "sha256", encoding = "hex") =>
    createHash(algorithm).update(bytes).digest(encoding);
  const source = join(temporary, "source");
  const files = {
    "bin/dsoffice": "native binary fixture",
    "program/resources": "Office resources",
    LICENSE: "license fixture",
    "sources/worker.cxx": "source fixture",
  };
  for (const [name, bytes] of Object.entries(files))
    await write(join(source, "package", name), bytes);
  await chmod(join(source, "package/bin/dsoffice"), 0o755);
  const manifest = JSON.stringify({
    files: Object.fromEntries(
      Object.entries(files).map(([name, bytes]) => [name, hash(bytes)]),
    ),
  });
  await write(join(source, "package/prebuilds.json"), manifest);
  const cache = join(temporary, "cache");
  await mkdir(cache);
  const archive = join(cache, "office.tgz");
  execFileSync("tar", ["-czf", archive, "-C", source, "package"]);
  const release = {
    version: "1.2.3",
    executable: "bin/dsoffice",
    targets: {
      arm64: {
        url: "https://registry.npmjs.org/office.tgz",
        integrity: `sha512-${hash(await readFile(archive), "sha512", "base64")}`,
        manifestSHA256: hash(manifest),
        executableSHA256: hash(files["bin/dsoffice"]),
      },
    },
  };
  await write(
    join(temporary, "plugins/deliverables/backend/office-release.json"),
    JSON.stringify(release),
  );
  const script = join(temporary, "tooling/office-toolchain.mjs");
  await mkdir(dirname(script));
  await copyFile(new URL("./office-toolchain.mjs", import.meta.url), script);
  const { stageOffice } = await import(pathToFileURL(script).href);
  const options = {
    arch: "arm64",
    output: join(temporary, "output"),
    cache,
    offline: true,
  };
  return { stageOffice, options, archive, release, files };
}

test("stages the complete verified release offline and verifies reuse", async (t) => {
  const { stageOffice, options, files } = await fixture(t);
  const executable = await stageOffice(options);
  const installation = dirname(dirname(executable));
  for (const [name, content] of Object.entries(files))
    assert.equal(await readFile(join(installation, name), "utf8"), content);
  assert.equal(await stageOffice(options), executable);
  await writeFile(join(installation, "program/resources"), "corrupt resources");
  await assert.rejects(stageOffice(options), /installation checksum mismatch/);
  assert.equal(
    await readFile(join(installation, "program/resources"), "utf8"),
    "corrupt resources",
  );
});

test("refuses corrupt cached archives before extraction", async (t) => {
  const { stageOffice, options, archive } = await fixture(t);
  await writeFile(archive, "corrupt archive");
  await assert.rejects(
    stageOffice(options),
    /prepared Office artifact checksum mismatch/,
  );
  assert.deepEqual(await readdir(options.output), []);
});

test("requires a supported architecture and an explicitly prepared offline archive", async (t) => {
  const { stageOffice, options, archive } = await fixture(t);
  await assert.rejects(
    stageOffice({ ...options, arch: "other" }),
    /no selected artifact/,
  );
  await rm(archive);
  await assert.rejects(
    stageOffice(options),
    /missing prepared Office artifact/,
  );
});
