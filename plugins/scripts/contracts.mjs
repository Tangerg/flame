import { execFileSync } from "node:child_process";
import {
  access,
  cp,
  mkdir,
  mkdtemp,
  readFile,
  rename,
  rm,
  writeFile,
} from "node:fs/promises";
import { basename, dirname, join, relative, sep } from "node:path";
import { pathToFileURL } from "node:url";
import { readBuildPlan, resourcePath } from "./build.mjs";

async function exists(path) {
  try {
    await access(path);
    return true;
  } catch (error) {
    if (error.code === "ENOENT") return false;
    throw error;
  }
}

async function contractSources(project, plan) {
  const managed = new Set(plan.managed.map((backend) => backend.id));
  const native = new Set(Object.keys(plan.mcp?.mcpServers ?? {}));
  const contracts = new Set();
  for (const contribution of plan.declaration.contributions ?? []) {
    if (managed.has(contribution.server))
      contracts.add(resourcePath(contribution.contract));
    else if (!native.has(contribution.server))
      throw new Error(
        "contract contribution requires its declared backend owner",
      );
  }
  const directories = new Set();
  for (const resource of contracts) {
    if (!resource.endsWith(".json"))
      throw new Error("published contract must be a JSON resource");
    const directory = dirname(resource);
    if (directories.has(directory))
      throw new Error(
        "independent contracts require separate projection directories",
      );
    directories.add(directory);
  }
  if (contracts.size > 1 && plan.declaration.components?.length)
    throw new Error(
      "component generation requires one contract projection owner",
    );
  return Promise.all(
    [...contracts].map(async (resource) => {
      const stem = basename(resource).replace(/(?:\.generated)?\.json$/, "");
      const json = join(project, "contracts", `${stem}.json`);
      const source = join(project, "contracts", `${stem}.ts`);
      const [hasJSON, hasSource] = await Promise.all([
        exists(json),
        exists(source),
      ]);
      if (hasJSON === hasSource)
        throw new Error(
          "each published contract requires exactly one JSON or TypeScript source",
        );
      return {
        resource,
        source: hasSource ? source : json,
        typescript: hasSource,
      };
    }),
  );
}

async function compileDescriptor(source, temporary) {
  // Keep the descriptor and exact JSON codec in one bundle so ExactNumber has one runtime identity.
  const { rolldown } = await import("rolldown");
  const entry = join(temporary, "source.mjs");
  const compiled = join(temporary, "contract.mjs");
  const specifier = relative(temporary, source).split(sep).join("/");
  await writeFile(
    entry,
    `import descriptor from ${JSON.stringify(specifier)};\n` +
      'import { stringifyJSON } from "@qimu/sdk/json";\n' +
      'if (!descriptor || typeof descriptor !== "object" || Array.isArray(descriptor))\n' +
      '  throw new Error("contract source must export its descriptor as default");\n' +
      "export default stringifyJSON(descriptor, true);\n",
  );
  const bundle = await rolldown({
    input: entry,
    platform: "node",
    resolve: { mainFields: ["module", "main"] },
    external: (id) => id.startsWith("node:"),
  });
  try {
    await bundle.write({ file: compiled, format: "esm" });
  } finally {
    await bundle.close();
  }
  const encoded = (await import(pathToFileURL(compiled).href)).default;
  const feature = join(temporary, "feature.json");
  await writeFile(feature, encoded);
  return feature;
}

export async function generateContracts(project, tool) {
  const plan = await readBuildPlan(project);
  const contracts = await contractSources(project, plan);
  for (const { resource, source, typescript } of contracts) {
    const temporary = await mkdtemp(join(project, ".flame-contract-"));
    try {
      let feature = source;
      if (typescript) feature = await compileDescriptor(source, temporary);
      else JSON.parse(await readFile(source, "utf8"));
      const output = join(temporary, "package");
      const component = plan.declaration.components?.length
        ? join(temporary, "component")
        : undefined;
      execFileSync(
        tool.command,
        [
          ...(tool.args ?? []),
          "contract",
          "--feature",
          feature,
          "--out",
          output,
          ...(component ? ["--ts", component] : []),
        ],
        {
          cwd: tool.cwd ?? project,
          stdio: "inherit",
          env: tool.env ?? process.env,
        },
      );
      if (basename(resource) !== "capability.generated.json") {
        const target = join(output, basename(resource));
        if (await exists(target))
          throw new Error(
            "contract resource collides with another generated projection",
          );
        await rename(join(output, "capability.generated.json"), target);
      }
      const destination = dirname(join(project, "package", resource));
      await mkdir(destination, { recursive: true });
      await cp(output, destination, { recursive: true });
      if (component) {
        await mkdir(join(project, "component"), { recursive: true });
        await cp(component, join(project, "component"), { recursive: true });
      }
    } finally {
      await rm(temporary, { recursive: true, force: true });
    }
  }
}
