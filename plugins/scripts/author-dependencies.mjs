import { readFile, readdir } from "node:fs/promises";
import { isBuiltin } from "node:module";
import { extname, join } from "node:path";
import { parseSync, transform } from "rolldown/utils";

function packageName(specifier) {
  if (isBuiltin(specifier) || specifier.startsWith(".")) return;
  const parts = specifier.split("/");
  const name = parts.slice(0, specifier.startsWith("@") ? 2 : 1).join("/");
  if (!name || name.endsWith("/"))
    throw new Error(`invalid author package import: ${specifier}`);
  return name;
}

function testEnvironment(program, comments) {
  const start = program.body[0]?.start ?? Infinity;
  for (const comment of comments) {
    if (comment.end > start) break;
    const name = comment.value.match(
      /(?:^|\n)\s*\*?\s*@(?:vitest|jest)-environment\s+([\w-]+)\b/u,
    )?.[1];
    if (!name) continue;
    // Vitest's public environment contract owns these provider names.
    // https://vitest.dev/guide/environment
    switch (name) {
      case "node":
        return;
      case "jsdom":
      case "happy-dom":
        return name;
      case "edge-runtime":
        return "@edge-runtime/vm";
      default:
        return `vitest-environment-${name}`;
    }
  }
}

function fileDependencies(code, filename, lang, test = false) {
  const found = new Set();
  const add = (source) => {
    if (source?.type !== "Literal" || typeof source.value !== "string")
      throw new Error(
        `author import must name a build-time module: ${filename}`,
      );
    const name = packageName(source.value);
    if (name) found.add(name);
  };
  function visit(node) {
    if (!node || typeof node !== "object") return;
    switch (node.type) {
      case "ImportDeclaration":
      case "ExportAllDeclaration":
        add(node.source);
        break;
      case "ExportNamedDeclaration":
        if (node.source) add(node.source);
        break;
      case "ImportExpression":
        add(node.source);
        break;
      case "TSImportType":
        add(node.source);
        break;
      case "TSExternalModuleReference":
        add(node.expression);
        break;
      case "CallExpression":
        if (
          node.callee.type === "Identifier" &&
          node.callee.name === "require"
        ) {
          if (node.arguments.length !== 1)
            throw new Error(`invalid author require: ${filename}`);
          add(node.arguments[0]);
        }
        break;
    }
    for (const [key, child] of Object.entries(node)) {
      if (["loc", "start", "end"].includes(key)) continue;
      if (Array.isArray(child)) child.forEach(visit);
      else visit(child);
    }
  }
  const parsed = parseSync(filename, code, { lang });
  if (parsed.errors.length)
    throw new Error(`author source parsing failed: ${filename}`, {
      cause: parsed.errors,
    });
  visit(parsed.program);
  if (test) {
    const environment = testEnvironment(parsed.program, parsed.comments);
    if (environment) found.add(environment);
  }
  return found;
}

async function sourceDependencies(source) {
  const found = { production: new Set(), tests: new Set(), hasTests: false };
  async function visit(directory) {
    let entries;
    try {
      entries = await readdir(directory, { withFileTypes: true });
    } catch (error) {
      if (error.code === "ENOENT") return;
      throw error;
    }
    for (const entry of entries) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) {
        await visit(path);
        continue;
      }
      if (
        !entry.isFile() ||
        !/\.[cm]?[jt]sx?$/.test(entry.name) ||
        /\.generated\.[cm]?[jt]sx?$/.test(entry.name)
      )
        continue;
      const selected = /\.(?:test|spec)\.[cm]?[jt]sx?$/.test(entry.name)
        ? found.tests
        : found.production;
      if (selected === found.tests) found.hasTests = true;
      const suffix = extname(path);
      const lang = suffix.endsWith("tsx")
        ? "tsx"
        : suffix.endsWith("jsx")
          ? "jsx"
          : suffix.endsWith("ts")
            ? "ts"
            : "js";
      const code = await readFile(path, "utf8");
      for (const name of fileDependencies(
        code,
        path,
        lang,
        selected === found.tests,
      ))
        selected.add(name);
      if (lang === "tsx" || lang === "jsx") {
        const lowered = await transform(path, code, {
          jsx: { runtime: "automatic", development: false },
        });
        if (lowered.errors.length)
          throw new Error(`author JSX transformation failed: ${path}`);
        for (const name of fileDependencies(lowered.code, path, "js"))
          selected.add(name);
      }
    }
  }
  for (const directory of ["backend", "component", "contracts"])
    await visit(join(source, directory));
  return found;
}

export async function authorDependencies({
  source,
  manifest,
  lock,
  packages,
  required,
}) {
  const roots = await sourceDependencies(source);
  for (const name of required) roots.production.add(name);
  for (const name of roots.production)
    if (!Object.hasOwn(manifest.dependencies ?? {}, name))
      throw new Error(`author source imports undeclared dependency: ${name}`);
  for (const name of roots.tests)
    if (
      !Object.hasOwn(manifest.dependencies ?? {}, name) &&
      !Object.hasOwn(manifest.devDependencies ?? {}, name)
    )
      throw new Error(`author test requires undeclared dependency: ${name}`);

  const candidates = new Map(
    packages.map((entry) => [entry.manifest.name, entry]),
  );
  const dependencies = {};
  const devDependencies = {};
  const selected = [];
  const visiting = new Set();
  const visited = new Set();
  const version = (name) => {
    const entry = lock.packages[`node_modules/${name}`];
    if (!entry?.version || entry.link)
      throw new Error(`root lock has no packaged selection for ${name}`);
    return entry.version;
  };
  function select(name, development) {
    if (visited.has(name)) return;
    const candidate = candidates.get(name);
    if (!candidate) {
      (development ? devDependencies : dependencies)[name] = version(name);
      visited.add(name);
      return;
    }
    if (visiting.has(name))
      throw new Error(`author package dependency cycle: ${name}`);
    visiting.add(name);
    for (const dependency of Object.keys(
      candidate.manifest.dependencies ?? {},
    )) {
      if (candidates.has(dependency)) select(dependency, development);
      else version(dependency);
    }
    for (const peer of Object.keys(candidate.manifest.peerDependencies ?? {}))
      if (!candidate.manifest.peerDependenciesMeta?.[peer]?.optional)
        select(peer, development);
    visiting.delete(name);
    visited.add(name);
    selected.push({ ...candidate, development });
  }
  for (const name of [...roots.production].sort()) select(name, false);
  for (const name of [...roots.tests].sort()) select(name, true);
  return {
    dependencies,
    devDependencies,
    packages: selected,
    hasTests: roots.hasTests,
  };
}
