import { existsSync, readdirSync, readFileSync, realpathSync } from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { API } from "typescript/unstable/sync";
import { transformSync, traverse } from "@babel/core";
import typescriptPreset from "@babel/preset-typescript";
import * as ts from "typescript/unstable/ast";

export const RUNTIME_CLIENT_EDGE = "@flame/runtime-contract/client";
const runtimeClientRoot = dirname(
  realpathSync(fileURLToPath(import.meta.resolve(RUNTIME_CLIENT_EDGE))),
);

export function sourceFiles(root) {
  const files = [];
  function walk(directory) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (/\.(?:[cm]?ts|tsx)$/.test(entry.name)) files.push(path);
    }
  }
  walk(root);
  if (files.length === 0) throw new Error(`no TypeScript sources in ${root}`);
  return files.sort();
}

export function assertSourceCoverage(expected, actual) {
  const covered = new Set(actual.map((path) => resolve(path)));
  const missing = expected.filter((path) => !covered.has(resolve(path)));
  if (missing.length) throw new Error(`source scan omitted:\n${missing.join("\n")}`);
}

export function visit(node, callback) {
  callback(node);
  node.forEachChild((child) => visit(child, callback));
}

function importEdges(source) {
  const edges = [];
  visit(source, (node) => {
    if (ts.isImportDeclaration(node)) {
      edges.push(node.moduleSpecifier);
    } else if (ts.isExportDeclaration(node) && node.moduleSpecifier) {
      edges.push(node.moduleSpecifier);
    } else if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument)) {
      edges.push(node.argument.literal);
    } else if (
      ts.isCallExpression(node) &&
      (node.expression.kind === ts.SyntaxKind.ImportKeyword ||
        (ts.isIdentifier(node.expression) && node.expression.text === "require"))
    ) {
      const specifier = node.arguments[0];
      if (specifier && ts.isStringLiteralLikeNode(specifier)) {
        edges.push(specifier);
      }
    } else if (
      ts.isImportEqualsDeclaration(node) &&
      ts.isExternalModuleReference(node.moduleReference)
    ) {
      edges.push(node.moduleReference.expression);
    }
  });
  return edges;
}

function runtimeSpecifiers(path) {
  if (/\.d\.[cm]?ts$/.test(path)) return new Set();
  // Erase TypeScript before collecting runtime edges. An ordinary import used
  // only in type positions disappears from the emitted module.
  const { ast } = transformSync(readFileSync(path, "utf8"), {
    filename: path,
    babelrc: false,
    configFile: false,
    parserOpts: { plugins: path.endsWith(".tsx") ? ["jsx"] : [] },
    presets: [[typescriptPreset, { onlyRemoveTypeImports: false }]],
    ast: true,
    code: false,
  });
  const modules = new Set();
  const recordStaticSpecifier = (node) => {
    if (node?.type === "StringLiteral") {
      modules.add(node.value);
    } else if (node?.type === "TemplateLiteral" && node.expressions.length === 0) {
      const value = node.quasis[0]?.value.cooked;
      if (typeof value === "string") modules.add(value);
    }
  };
  traverse(ast, {
    ImportDeclaration({ node }) {
      modules.add(node.source.value);
    },
    ExportNamedDeclaration({ node }) {
      if (node.source) modules.add(node.source.value);
    },
    ExportAllDeclaration({ node }) {
      modules.add(node.source.value);
    },
    ImportExpression({ node }) {
      recordStaticSpecifier(node.source);
    },
    CallExpression({ node }) {
      if (
        node.callee.type === "Import" ||
        (node.callee.type === "Identifier" && node.callee.name === "require")
      ) {
        recordStaticSpecifier(node.arguments[0]);
      }
    },
  });
  return modules;
}

export function withSourceProject(root, inspect) {
  const src = resolve(root, "src");
  const expected = sourceFiles(src);
  const compiler = new API({ cwd: root });
  try {
    const config = resolve(root, "tsconfig.json");
    const project = compiler.updateSnapshot({ openProjects: [config] }).getProject(config);
    if (!project) throw new Error(`TypeScript did not load ${config}`);
    const { program } = project;
    assertSourceCoverage(expected, program.getSourceFileNames());
    return inspect(project, expected);
  } finally {
    compiler.close();
  }
}

export function readSourceGraph(root = process.cwd()) {
  const src = resolve(root, "src");
  return withSourceProject(root, (project, expected) => {
    const { program, checker } = project;
    const graph = {};
    const values = {};
    const localName = (path) => relative(src, resolve(path)).split(sep).join("/");
    for (const path of expected) {
      const source = program.getSourceFile(path);
      if (!source) throw new Error(`TypeScript did not load ${path}`);
      if (program.getSyntacticDiagnostics(path).length) {
        throw new Error(`cannot inspect malformed TypeScript: ${path}`);
      }
      const dependencies = new Set();
      const runtimeDependencies = new Set();
      const emitted = runtimeSpecifiers(path);
      for (const specifier of importEdges(source)) {
        if (!specifier || !ts.isStringLiteralLikeNode(specifier)) continue;
        const name = specifier.text;
        let target;
        if (name === RUNTIME_CLIENT_EDGE || name.startsWith(`${RUNTIME_CLIENT_EDGE}/`)) {
          target = RUNTIME_CLIENT_EDGE;
        } else {
          const symbol = checker.getSymbolAtLocation(specifier);
          const declarations = (symbol?.declarations ?? []).map((handle) =>
            handle.resolve(project),
          );
          const declaration = declarations.find((entry) => entry && ts.isSourceFile(entry));
          if (declaration) {
            const resolved = resolve(declaration.fileName);
            // Relative paths and aliases must retain the SDK's ownership even
            // though its implementation lives outside the frontend source tree.
            if (realpathSync(resolved).startsWith(`${runtimeClientRoot}${sep}`)) {
              target = RUNTIME_CLIENT_EDGE;
            } else if (resolved.startsWith(`${src}${sep}`)) {
              target = localName(resolved);
            }
          } else if (name.startsWith(".") || name.startsWith("@/")) {
            // Asset modules resolve through ambient Vite declarations. They still
            // have to name a real file; unresolved source imports never disappear.
            const asset = name.startsWith("@/")
              ? resolve(src, name.slice(2))
              : resolve(path, "..", name);
            if (/\.(?:[cm]?[jt]sx?)$/.test(name) || !existsSync(asset)) {
              throw new Error(`unresolved local import: ${localName(path)} -> ${name}`);
            }
          }
        }
        if (target) {
          dependencies.add(target);
          if (emitted.has(name)) runtimeDependencies.add(target);
        }
      }
      graph[localName(path)] = [...dependencies];
      values[localName(path)] = [...runtimeDependencies];
    }
    return { graph, values };
  });
}

export function findCycles(graph) {
  const cycles = [];
  const visited = new Set();
  const active = new Set();
  const path = [];
  function walk(node) {
    if (active.has(node)) {
      cycles.push([...path.slice(path.indexOf(node)), node]);
      return;
    }
    if (visited.has(node)) return;
    visited.add(node);
    active.add(node);
    path.push(node);
    for (const dependency of graph[node] ?? []) walk(dependency);
    path.pop();
    active.delete(node);
  }
  for (const node of Object.keys(graph)) walk(node);
  return cycles;
}
