#!/usr/bin/env node
import { relative, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { SymbolFlags } from "typescript/unstable/sync";
import * as ts from "typescript/unstable/ast";
import { visit, withSourceProject } from "../../../tooling/source-graph.mjs";

const isTest = (path) =>
  /\.(test|spec)\.(?:[cm]?ts|tsx)$/.test(path) ||
  path.startsWith("test/") ||
  path.includes("testkit");

const retiredAPIs = new Set(["SingletonPort", "createSingletonPort"]);

function moduleSpecifier(node) {
  if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) return node.moduleSpecifier;
  if (ts.isImportTypeNode(node) && ts.isLiteralTypeNode(node.argument))
    return node.argument.literal;
  if (
    ts.isCallExpression(node) &&
    (node.expression.kind === ts.SyntaxKind.ImportKeyword ||
      (ts.isIdentifier(node.expression) && node.expression.text === "require"))
  )
    return node.arguments[0];
  if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference))
    return node.moduleReference.expression;
}

export function inspectPortSurface(root = process.cwd()) {
  return withSourceProject(root, (project, paths) => {
    const { program, checker } = project;
    const src = resolve(root, "src");
    const sources = paths
      .filter((path) => !isTest(relative(src, path)))
      .map((path) => program.getSourceFile(path));
    const clauses = new Map();
    const used = new Set();
    const retired = new Set();
    const key = (node) => `${node.getSourceFile().fileName}:${node.getStart()}`;
    const declarations = (symbol) => {
      if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      return (symbol?.declarations ?? []).map((handle) => handle.resolve(project)).filter(Boolean);
    };
    const record = (symbol, target) => {
      for (const declaration of declarations(symbol)) target.add(key(declaration));
    };

    for (const source of sources) {
      const path = relative(src, source.fileName);
      visit(source, (node) => {
        if (ts.isIdentifier(node) && retiredAPIs.has(node.text)) {
          retired.add(`${path}  ${node.text}`);
        }
        const specifier = moduleSpecifier(node);
        if (
          specifier &&
          ts.isStringLiteralLikeNode(specifier) &&
          /(?:^|\/)singletonPort(?:\.[cm]?[jt]s)?$/.test(specifier.text)
        ) {
          retired.add(`${path}  ${specifier.text}`);
        }
        if (ts.isInterfaceDeclaration(node) && node.name.text.endsWith("Port")) {
          for (const member of node.members) {
            if (
              ts.isMethodSignatureDeclaration(member) ||
              (ts.isPropertySignatureDeclaration(member) &&
                member.type &&
                ts.isFunctionTypeNode(member.type))
            ) {
              clauses.set(key(member), `${path}  ${node.name.text}.${member.name.getText()}()`);
            }
          }
        }
        if (ts.isPropertyAccessExpression(node)) {
          record(checker.getSymbolAtLocation(node.name), used);
        } else if (
          ts.isElementAccessExpression(node) &&
          ts.isStringLiteralLikeNode(node.argumentExpression)
        ) {
          record(
            checker.getPropertyOfType(
              checker.getTypeAtLocation(node.expression),
              node.argumentExpression.text,
            ),
            used,
          );
        } else if (ts.isBindingElement(node) && ts.isObjectBindingPattern(node.parent)) {
          const name = node.propertyName ?? node.name;
          if (ts.isIdentifier(name) || ts.isStringLiteralLikeNode(name)) {
            record(
              checker.getPropertyOfType(checker.getTypeAtLocation(node.parent), name.text),
              used,
            );
          }
        }
      });
    }
    return {
      files: paths.length,
      dead: [...clauses].filter(([id]) => !used.has(id)).map(([, label]) => label),
      retired: [...retired],
    };
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { files, dead, retired } = inspectPortSurface();
  if (dead.length || retired.length) {
    console.error("check-port-surface: invalid application contracts:");
    for (const entry of dead) console.error(`  port clause: ${entry}`);
    for (const entry of retired)
      console.error(`  retired singleton API: ${entry}; capture an explicit feature instance`);
    process.exit(1);
  }
  console.log(
    `check-port-surface: ${files} sources covered; every port clause has a production consumer; no retired singleton APIs`,
  );
}
