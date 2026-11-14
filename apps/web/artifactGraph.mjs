import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { isAbsolute, join, resolve } from "node:path";

export const ARTIFACT_GRAPH = ".vite/flame-artifact-graph.json";
const STARTUP_BUDGETS = { js: 2_785_000, css: 147_000 };
export const LAZY_BUDGETS = {
  settings: { label: "settings panes", kind: "module", ceiling: 200_000 },
  workspace: { label: "workspace views", kind: "module", ceiling: 250_000 },
  "syntax-highlighting": { label: "syntax highlighting", kind: "module", ceiling: 3_000_000 },
  "diagram-rendering": { label: "diagram rendering", kind: "module", ceiling: 3_000_000 },
  "math-styles": { label: "math stylesheet", kind: "css", ceiling: 100_000 },
};
const BOOTSTRAP_FORBIDDEN = new Set([
  "react",
  "react-dom",
  "dougong",
  "@flame/component-sdk",
  "@flame/runtime-client",
  "@flame/backend-sdk",
  "@frame/sdk",
]);
export const artifactDigest = (bytes) => createHash("sha256").update(bytes).digest("hex");

export function isStylesheetRequest(id) {
  return (
    id.split("?")[0].endsWith(".css") &&
    !/[?&](?:worker|sharedworker|raw|url|inline|transform-only)\b/.test(id) &&
    !/[?&]commonjs-proxy/.test(id)
  );
}

export function checkArtifactPath(path) {
  if (
    !path ||
    isAbsolute(path) ||
    path.includes("\\") ||
    path.split("/").some((part) => !part || part === "." || part === "..")
  )
    throw new Error(`invalid artifact path: ${path}`);
}

function moduleFiles(graph, source) {
  return Object.values(graph.files)
    .filter((file) => file.facade === source || file.modules?.includes(source))
    .map((file) => file.path);
}

function staticClosure(graph, roots) {
  const js = new Set();
  const css = new Set();
  const visit = (path) => {
    if (js.has(path)) return;
    const file = graph.files[path];
    if (!file) throw new Error(`missing artifact: ${path}`);
    js.add(path);
    for (const dependency of file.imports ?? [])
      visit(graph.document.imports[dependency] ?? dependency);
    for (const stylesheet of file.css ?? []) {
      if (!graph.files[stylesheet]) throw new Error(`missing stylesheet: ${stylesheet}`);
      css.add(stylesheet);
    }
  };
  for (const path of roots) visit(path);
  return { js, css };
}

export function analyzeArtifactGraph(graph) {
  const errors = [];
  const document = graph.document;
  const starts = graph.declarations.filter((entry) => entry.kind === "startup");
  if (starts.length !== 1) errors.push("the document needs one declared immediate startup import");
  const bootstrap = staticClosure(graph, [
    ...document.scripts.map((script) => script.path),
    ...document.modulepreloads,
  ]);
  for (const path of document.stylesheets) bootstrap.css.add(path);
  const bootstrapModules = new Set(
    [...bootstrap.js].flatMap((path) => graph.files[path].modules ?? []),
  );
  for (const path of bootstrap.js) {
    const file = graph.files[path];
    for (const source of file.modules ?? []) {
      if (!Object.hasOwn(file.modulePackages ?? {}, source))
        errors.push(`bootstrap module has no package identity: ${source}`);
      const name = file.modulePackages?.[source];
      if (BOOTSTRAP_FORBIDDEN.has(name) && source !== graph.browserPolicy)
        errors.push(`bootstrap-only graph contains ${name} source ${source} in ${path}`);
    }
  }
  for (const entry of starts) {
    if (!bootstrapModules.has(entry.importer))
      errors.push(`startup importer is outside the bootstrap graph: ${entry.importer}`);
    if (bootstrapModules.has(entry.source))
      errors.push(
        `the immediate main source is already in the bootstrap-only graph: ${entry.source}`,
      );
  }
  const immediate = starts.flatMap((entry) => {
    const files = moduleFiles(graph, entry.source);
    if (!files.length) errors.push(`startup source was not emitted: ${entry.source}`);
    return files;
  });
  const startup = staticClosure(graph, [...bootstrap.js, ...immediate]);
  for (const script of document.scripts) startup.js.add(script.path);
  for (const path of document.stylesheets) startup.css.add(path);
  const startupModules = new Set(
    [...startup.js]
      .flatMap((path) => [graph.files[path]?.facade, ...(graph.files[path]?.modules ?? [])])
      .filter(Boolean),
  );
  const requested = new Set([
    ...startup.js,
    ...startup.css,
    ...document.preload,
    ...document.prefetch,
  ]);
  const size = (paths) =>
    [...paths].reduce((total, path) => {
      if (!graph.files[path]) throw new Error(`missing artifact: ${path}`);
      return total + graph.files[path].bytes;
    }, 0);
  const budgets = Object.entries(STARTUP_BUDGETS).map(([kind, ceiling]) => ({
    label: kind.toUpperCase(),
    used: size(startup[kind]),
    ceiling,
  }));
  const features = [];
  for (const [group, policy] of Object.entries(LAZY_BUDGETS)) {
    const declarations = graph.declarations.filter(
      (entry) => entry.kind === "lazy" && entry.group === group,
    );
    if (!declarations.length) errors.push(`${policy.label}: no declared lazy source found`);
    const roots = new Set();
    for (const entry of declarations) {
      if (!moduleFiles(graph, entry.importer).length)
        errors.push(`${policy.label}: declaring source was not emitted: ${entry.importer}`);
      if (policy.kind === "module") {
        const files = moduleFiles(graph, entry.source);
        if (!files.length) errors.push(`${policy.label}: no chunk found for ${entry.source}`);
        if (startupModules.has(entry.source) || files.some((path) => requested.has(path)))
          errors.push(
            `${policy.label}: declared source is on the startup or HTML request path: ${entry.source}`,
          );
        for (const path of files) roots.add(path);
      } else {
        if (!graph.cssContains) errors.push(`${policy.label}: CSS inclusion provenance is missing`);
        const visitedCss = new Set();
        const inspectCss = (source) => {
          if (visitedCss.has(source)) return;
          visitedCss.add(source);
          const sourceImporters = graph.cssImporters?.[source];
          if (!Array.isArray(sourceImporters) || sourceImporters.length === 0) {
            errors.push(`${policy.label}: no CSS source provenance for ${source}`);
            return;
          }
          for (const importer of sourceImporters) {
            if (startupModules.has(importer))
              errors.push(`${policy.label}: CSS source ${source} has startup importer ${importer}`);
            if (Object.hasOwn(graph.cssImporters, importer)) inspectCss(importer);
          }
          for (const [owner, dependencies] of Object.entries(graph.cssContains ?? {}))
            if (dependencies.includes(source)) inspectCss(owner);
        };
        inspectCss(entry.source);
        const importers = moduleFiles(graph, entry.importer);
        const stylesheets = importers.flatMap((path) => graph.files[path].css ?? []);
        if (!stylesheets.length)
          errors.push(`${policy.label}: no emitted CSS mapping for ${entry.source}`);
        for (const path of stylesheets) {
          if (requested.has(path))
            errors.push(
              `${policy.label}: index.html references or startup imports ${path} — it must stay lazy`,
            );
          roots.add(path);
        }
      }
    }
    const closure =
      policy.kind === "module" ? staticClosure(graph, roots) : { js: new Set(), css: roots };
    const full = new Set([...closure.js, ...closure.css]);
    const shared = new Set(
      [...full].filter((path) => startup.js.has(path) || startup.css.has(path)),
    );
    const increment = new Set([...full].filter((path) => !shared.has(path)));
    const feature = {
      group,
      label: policy.label,
      roots: [...roots],
      files: [...full],
      shared: [...shared],
      increment: [...increment],
      fullBytes: size(full),
      sharedBytes: size(shared),
      used: size(increment),
      ceiling: policy.ceiling,
    };
    features.push(feature);
    budgets.push(feature);
  }
  return {
    bootstrap: { js: [...bootstrap.js], css: [...bootstrap.css] },
    startup: { js: [...startup.js], css: [...startup.css] },
    preload: document.preload,
    prefetch: document.prefetch,
    budgets,
    features,
    errors,
  };
}

export function readArtifactGraph(directory) {
  const graph = JSON.parse(readFileSync(join(directory, ARTIFACT_GRAPH), "utf8"));
  for (const [path, file] of Object.entries(graph.files)) {
    checkArtifactPath(path);
    if (file.path !== path) throw new Error(`artifact identity disagrees with its path: ${path}`);
    for (const key of ["imports", "dynamicImports", "css", "assets"])
      for (const dependency of file[key] ?? []) {
        checkArtifactPath(dependency);
        if (!Object.hasOwn(graph.files, dependency))
          throw new Error(`missing emitted dependency: ${dependency}`);
      }
    const absolute = resolve(directory, path);
    const bytes = readFileSync(absolute);
    if (bytes.length !== file.bytes || artifactDigest(bytes) !== file.sha256)
      throw new Error(`artifact graph is stale: ${path}`);
  }
  return graph;
}
