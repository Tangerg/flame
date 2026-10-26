import assert from "node:assert/strict";
import { rolldown } from "rolldown";
import { access, mkdtemp, readdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { buildComponent } from "@flame/component-sdk/build";

const root = new URL("../", import.meta.url);
for (const directory of await readdir(root, { withFileTypes: true })) {
  if (!directory.isDirectory()) continue;
  const source = new URL(`${directory.name}/component/component.tsx`, root);
  try {
    await access(source);
  } catch (error) {
    if (error.code === "ENOENT") continue;
    throw error;
  }
  const manifest = JSON.parse(
    await readFile(
      new URL(`${directory.name}/package/plugin.json`, root),
      "utf8",
    ),
  );
  const declaration = JSON.parse(
    await readFile(
      new URL(
        `${directory.name}/package/${manifest.extensions["io.github.tangerg.flame"].declaration}`,
        root,
      ),
      "utf8",
    ),
  );
  assert.ok(declaration.components.length > 0);
  assert.equal(
    new Set(declaration.components.map((entry) => entry.entry)).size,
    1,
  );
  const target = dirname(
    fileURLToPath(
      new URL(
        `${directory.name}/package/${declaration.components[0].entry}`,
        root,
      ),
    ),
  );
  const check = process.argv.includes("--check");
  if (!check) await rm(target, { recursive: true, force: true });
  const output = check
    ? await mkdtemp(join(tmpdir(), "flame-components-"))
    : target;
  try {
    const built = await buildComponent({
      input: fileURLToPath(source),
      directory: output,
      compiler: rolldown,
    });
    assert.deepEqual(
      [...built.exports].sort(),
      [...new Set(declaration.components.map((entry) => entry.export))].sort(),
      `${directory.name}: component exports must match declared contributions`,
    );
    if (check) {
      const files = (await readdir(output)).sort();
      assert.deepEqual((await readdir(target)).sort(), files);
      for (const file of files)
        assert.deepEqual(
          await readFile(join(target, file)),
          await readFile(join(output, file)),
          `rebuild the ${directory.name} component`,
        );
    }
  } finally {
    if (check) await rm(output, { recursive: true, force: true });
  }
  console.log(`${directory.name}: prebuilt ESM`);
}
