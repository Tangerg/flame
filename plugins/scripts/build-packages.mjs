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
    "archive-tool": { type: "string" },
    replace: { type: "boolean", default: false },
  },
});
await validateDestination(source, values.output);
// Resolve the selection to a validated build plan before any output is touched.
// Enumerating all plugins skips ordinary directories; an explicit --plugin must
// name a real plugin and the plan must be non-empty, so a missing or non-plugin
// selection cannot replace an existing release with an empty directory.
const plugins = [];
for (const entry of await readdir(source, { withFileTypes: true })) {
  if (!entry.isDirectory()) continue;
  try {
    await access(join(source, entry.name, "package/plugin.json"));
  } catch (error) {
    if (error.code === "ENOENT") continue;
    throw error;
  }
  plugins.push(entry.name);
}
let plan = plugins;
if (values.plugin !== undefined) {
  if (!plugins.includes(values.plugin)) {
    throw new Error(
      `no plugin named ${JSON.stringify(values.plugin)}; available: ${plugins.join(", ")}`,
    );
  }
  plan = [values.plugin];
}
if (plan.length === 0) {
  throw new Error("no plugins found to build");
}
await mkdir(dirname(values.output), { recursive: true });
const stage = await mkdtemp(join(dirname(values.output), ".flame-plugins-"));
try {
  const target = join(stage, "plugins");
  await mkdir(target);
  for (const name of plan) {
    await buildPlugin({
      ...values,
      source: join(source, name),
      output: join(target, name),
      archiveTool: values["archive-tool"],
    });
  }
  if (values.replace) await rm(values.output, { recursive: true, force: true });
  await rename(target, values.output);
} finally {
  await rm(stage, { recursive: true, force: true });
}
