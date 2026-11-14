import { parseSync } from "@babel/core";
import { parse } from "parse5";
import { preprocessCSS } from "vite";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, isAbsolute, join, relative, resolve } from "node:path";
import {
  ARTIFACT_GRAPH,
  LAZY_BUDGETS,
  artifactDigest,
  checkArtifactPath,
  isStylesheetRequest,
} from "./artifactGraph.mjs";

const slash = (path) => path.replaceAll("\\", "/");

function artifactPath(url, document = "index.html", base = "/") {
  const address = new URL(url, `https://flame.invalid/${document}`);
  if (address.origin !== "https://flame.invalid") throw new Error(`external artifact: ${url}`);
  const pathname = decodeURIComponent(address.pathname);
  const prefix = base.startsWith("/") ? base : "/";
  if (!pathname.startsWith(prefix)) throw new Error(`artifact is outside the build base: ${url}`);
  return pathname.slice(prefix.length);
}

export function artifactDocument(html, base = "/") {
  const document = {
    scripts: [],
    modulepreloads: [],
    stylesheets: [],
    preload: [],
    prefetch: [],
    imports: {},
  };
  const visit = (node) => {
    const attrs = Object.fromEntries((node.attrs ?? []).map(({ name, value }) => [name, value]));
    if (node.tagName === "script") {
      const type = (attrs.type ?? "").toLowerCase();
      if (type === "importmap") {
        Object.assign(
          document.imports,
          JSON.parse((node.childNodes ?? []).map((n) => n.value ?? "").join("")).imports,
        );
      } else if (["", "module", "text/javascript", "application/javascript"].includes(type)) {
        if (!attrs.src) throw new Error("executable inline scripts need an emitted module graph");
        document.scripts.push({
          path: artifactPath(attrs.src, "index.html", base),
          module: type === "module",
        });
      }
    }
    if (node.tagName === "link" && attrs.href) {
      const rel = (attrs.rel ?? "").toLowerCase().split(/\s+/);
      for (const [token, key] of [
        ["modulepreload", "modulepreloads"],
        ["stylesheet", "stylesheets"],
        ["preload", "preload"],
        ["prefetch", "prefetch"],
      ])
        if (rel.includes(token)) document[key].push(artifactPath(attrs.href, "index.html", base));
    }
    if (node.tagName !== "template") for (const child of node.childNodes ?? []) visit(child);
  };
  visit(parse(html));
  for (const [specifier, url] of Object.entries(document.imports))
    document.imports[specifier] = artifactPath(url, "index.html", base);
  return document;
}

function artifactDeclaration(note, importKind, specifier) {
  const group = note.startsWith("@flame-lazy:") ? note.slice("@flame-lazy:".length) : undefined;
  if (note !== "@flame-startup" && !Object.hasOwn(LAZY_BUDGETS, group))
    throw new Error(`unknown artifact annotation: ${note}`);
  if (!group && importKind !== "ImportExpression")
    throw new Error("startup annotations require an immediate dynamic import");
  return { specifier, kind: group ? "lazy" : "startup", ...(group ? { group } : {}) };
}

/** @returns {import("vite").Plugin} */
export function artifactGraph() {
  let config;
  let browserPolicy;
  const declarations = new Map();
  const cssSources = new Map();
  const cssDependencies = new Map();
  const packageNames = new Map();
  const sourceId = (id) => {
    if (id.startsWith("data:")) return `data:${artifactDigest(id)}`;
    const normalized = slash(id);
    const dependency = normalized.lastIndexOf("/node_modules/");
    if (dependency >= 0)
      return `${isAbsolute(id) ? "" : "virtual:"}npm:${normalized.slice(dependency + "/node_modules/".length)}`;
    if (isAbsolute(id)) return slash(relative(config.root, id));
    if (id.startsWith("\0") && isAbsolute(id.slice(1))) return `virtual:${sourceId(id.slice(1))}`;
    return `virtual:${artifactDigest(normalized.replaceAll(slash(config.root), "<root>"))}`;
  };
  const packageName = (id) => {
    if (!isAbsolute(id)) return null;
    let directory = dirname(id.split("?")[0]);
    const visited = [];
    while (!packageNames.has(directory)) {
      visited.push(directory);
      const path = join(directory, "package.json");
      if (existsSync(path)) {
        packageNames.set(directory, JSON.parse(readFileSync(path, "utf8")).name ?? null);
        break;
      }
      const parent = dirname(directory);
      if (parent === directory) {
        packageNames.set(directory, null);
        break;
      }
      directory = parent;
    }
    const name = packageNames.get(directory);
    for (const path of visited) packageNames.set(path, name);
    return name;
  };
  return {
    name: "flame-artifact-graph",
    apply: "build",
    enforce: "post",
    configResolved(value) {
      config = value;
    },
    async buildStart() {
      const policy = await this.resolve("@flame/component-sdk/browser-policy");
      if (!policy || policy.external) throw new Error("the public browser policy did not resolve");
      browserPolicy = sourceId(policy.id);
    },
    transform: {
      order: "pre",
      async handler(code, id) {
        if (isStylesheetRequest(id)) {
          // CSS @import files may never become bundler modules. These are inclusion facts, not asset bytes.
          const { deps } = await preprocessCSS(code, id, config);
          cssDependencies.set(id, [...(deps ?? [])].map(sourceId).toSorted());
          return;
        }
        if (!code.includes("@flame-")) return;
        const ast = parseSync(code, {
          filename: id,
          configFile: false,
          babelrc: false,
          parserOpts: { plugins: ["typescript", "jsx"], createImportExpressions: true },
        });
        const markers = (ast.comments ?? []).filter(
          (comment) =>
            comment.type === "CommentBlock" && comment.value.trim().startsWith("@flame-"),
        );
        const consumed = new Set();
        const imports = [];
        const visit = (node) => {
          if (!node || typeof node !== "object") return;
          const source =
            node.type === "ImportDeclaration" || node.type === "ImportExpression"
              ? node.source
              : undefined;
          if (source) {
            const notes = markers.filter(
              (comment) => comment.start >= node.start && comment.end <= source.start,
            );
            for (const comment of notes) {
              if (notes.length !== 1 || source.type !== "StringLiteral")
                throw new Error(`artifact annotation needs one literal import: ${id}`);
              consumed.add(comment);
              imports.push(artifactDeclaration(comment.value.trim(), node.type, source.value));
            }
          }
          for (const [key, child] of Object.entries(node)) {
            if (key === "loc" || key.endsWith("Comments") || key === "comments") continue;
            if (Array.isArray(child)) child.forEach(visit);
            else if (child && typeof child === "object") visit(child);
          }
        };
        visit(ast.program);
        if (consumed.size !== markers.length)
          throw new Error(`artifact annotation is not attached to an import: ${id}`);
        const resolved = [];
        for (const entry of imports) {
          const target = await this.resolve(entry.specifier, id, { skipSelf: true });
          if (!target || target.external)
            throw new Error(`artifact source did not resolve: ${entry.specifier}`);
          const css = isStylesheetRequest(target.id);
          if (entry.group && (LAZY_BUDGETS[entry.group].kind === "css") !== css)
            throw new Error(`artifact group and source type disagree: ${entry.specifier}`);
          if (css) cssSources.set(sourceId(target.id), target.id);
          resolved.push({
            kind: entry.kind,
            ...(entry.group ? { group: entry.group } : {}),
            importer: sourceId(id),
            source: sourceId(target.id),
          });
        }
        declarations.set(id, resolved);
      },
    },
    writeBundle(_options, bundle) {
      const chunks = new Map(
        Object.values(bundle)
          .filter((file) => file.type === "chunk")
          .map((file) => [
            file.fileName,
            {
              facade: file.facadeModuleId ? sourceId(file.facadeModuleId) : null,
              imports: file.imports,
              dynamicImports: file.dynamicImports,
              modules: Object.keys(file.modules).map(sourceId),
              modulePackages: Object.fromEntries(
                Object.keys(file.modules).map((id) => [sourceId(id), packageName(id)]),
              ),
              css: [...(file.viteMetadata?.importedCss ?? [])],
              assets: [...(file.viteMetadata?.importedAssets ?? [])],
            },
          ]),
      );
      const directory = resolve(config.root, config.build.outDir);
      const files = {};
      const walk = (parent) => {
        for (const entry of readdirSync(parent, { withFileTypes: true }).sort((left, right) =>
          left.name.localeCompare(right.name, "en"),
        )) {
          if (entry.name.startsWith(".")) continue;
          const absolute = join(parent, entry.name);
          if (entry.isDirectory()) walk(absolute);
          else if (entry.isFile()) {
            const path = slash(relative(directory, absolute));
            const bytes = readFileSync(absolute);
            files[path] = {
              path,
              bytes: bytes.length,
              sha256: artifactDigest(bytes),
              ...(chunks.get(path) ?? {}),
            };
          }
        }
      };
      walk(directory);
      const document = artifactDocument(
        readFileSync(join(directory, "index.html"), "utf8"),
        config.base,
      );
      const edge = (dependency, importer) => {
        let path = document.imports[dependency] ?? dependency;
        if (!Object.hasOwn(files, path) && path.startsWith("."))
          path = slash(join(dirname(importer), path));
        checkArtifactPath(path);
        if (!Object.hasOwn(files, path))
          throw new Error(`missing emitted dependency: ${dependency}`);
        return path;
      };
      for (const file of Object.values(files)) {
        for (const key of ["imports", "dynamicImports", "css", "assets"])
          if (file[key]) file[key] = file[key].map((path) => edge(path, file.path));
      }
      for (const path of [
        ...document.scripts.map((script) => script.path),
        ...document.modulepreloads,
        ...document.stylesheets,
        ...document.preload,
        ...document.prefetch,
        ...Object.values(document.imports),
      ])
        edge(path, "index.html");
      const cssImporters = {};
      const pendingCss = [...cssSources.values()];
      const cssContains = {};
      for (const [owner, dependencies] of cssDependencies) {
        const guarded = dependencies.filter((source) => cssSources.has(source));
        if (guarded.length) {
          cssContains[sourceId(owner)] = guarded;
          pendingCss.push(owner);
        }
      }
      const visitedCss = new Set();
      while (pendingCss.length) {
        const id = pendingCss.pop();
        if (visitedCss.has(id)) continue;
        visitedCss.add(id);
        const info = this.getModuleInfo(id);
        const importers = info?.importers.filter(
          (importer) => !importer.split("?")[0].endsWith(".css") || isStylesheetRequest(importer),
        );
        cssImporters[sourceId(id)] = importers ? importers.map(sourceId).toSorted() : null;
        for (const importer of importers ?? [])
          if (isStylesheetRequest(importer)) pendingCss.push(importer);
      }
      const graph = {
        document,
        files,
        browserPolicy,
        cssImporters,
        cssContains,
        declarations: [...declarations.values()]
          .flat()
          .sort(
            (left, right) =>
              left.importer.localeCompare(right.importer, "en") ||
              left.source.localeCompare(right.source, "en"),
          ),
      };
      const output = join(directory, ARTIFACT_GRAPH);
      mkdirSync(dirname(output), { recursive: true });
      writeFileSync(output, JSON.stringify(graph, null, 2) + "\n");
    },
  };
}
