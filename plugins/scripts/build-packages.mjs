import { mkdtemp, mkdir, readdir, access, rename, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import { buildPlugin, validateDestination } from "./build.mjs";
const source = fileURLToPath(new URL("../", import.meta.url));
const { values } = parseArgs({
  options: {
    output: { type: "string" },
    os: { type: "string" },
    arch: { type: "string" },
    plugin: { type: "string" },
    archives: { type: "string" },
    replace: { type: "boolean", default: false },
  },
});
await validateDestination(source, values.output);
await mkdir(dirname(values.output), { recursive: true });
const stage = await mkdtemp(join(dirname(values.output), ".flame-plugins-"));
try {
  const target = join(stage, "plugins");
  await mkdir(target);
  for (const entry of await readdir(source, { withFileTypes: true })) {
    if (!entry.isDirectory() || (values.plugin && values.plugin !== entry.name))
      continue;
    try {
      await access(join(source, entry.name, "package/plugin.json"));
    } catch (error) {
      if (error.code === "ENOENT") continue;
      throw error;
    }
    await buildPlugin({
      ...values,
      source: join(source, entry.name),
      output: join(target, entry.name),
    });
  }
  if (values.replace) await rm(values.output, { recursive: true, force: true });
  await rename(target, values.output);
} finally {
  await rm(stage, { recursive: true, force: true });
}
