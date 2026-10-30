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

function hasInstanceScope(node) {
  for (let owner = node.parent; owner; owner = owner.parent) {
    if (
      ts.isFunctionDeclaration(owner) ||
      ts.isFunctionExpression(owner) ||
      ts.isArrowFunction(owner) ||
      ts.isMethodDeclaration(owner) ||
      owner.kind === ts.SyntaxKind.Constructor ||
      owner.kind === ts.SyntaxKind.GetAccessor ||
      owner.kind === ts.SyntaxKind.SetAccessor
    )
      return true;
    if (
      ts.isPropertyDeclaration(owner) &&
      !owner.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.StaticKeyword)
    )
      return true;
  }
  return false;
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
    const ambient = new Set();
    const key = (node) => `${node.getSourceFile().fileName}:${node.getStart()}`;
    const declarations = (symbol) => {
      if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      return (symbol?.declarations ?? []).map((handle) => handle.resolve(project)).filter(Boolean);
    };
    const record = (symbol, target) => {
      for (const declaration of declarations(symbol)) target.add(key(declaration));
    };
    const isPublicationFactory = (expression, seen = new Set()) => {
      const symbol = checker.getSymbolAtLocation(
        ts.isPropertyAccessExpression(expression) ? expression.name : expression,
      );
      if (!symbol || seen.has(symbol)) return false;
      seen.add(symbol);
      return declarations(symbol).some((declaration) => {
        if (
          ts.isFunctionDeclaration(declaration) &&
          declaration.name?.text === "createPublicationSlot"
        )
          return true;
        return (
          ts.isVariableDeclaration(declaration) &&
          declaration.initializer &&
          isPublicationFactory(declaration.initializer, seen)
        );
      });
    };

    for (const source of sources) {
      const path = relative(src, source.fileName);
      visit(source, (node) => {
        if (ts.isIdentifier(node) && retiredAPIs.has(node.text)) {
          retired.add(`${path}  ${node.text}`);
        }
        if (
          ts.isCallExpression(node) &&
          !hasInstanceScope(node) &&
          isPublicationFactory(node.expression)
        ) {
          const { line } = source.getLineAndCharacterOfPosition(node.getStart());
          ambient.add(`${path}:${line + 1}  ${node.expression.getText()}()`);
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
      ambient: [...ambient],
    };
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { files, dead, retired, ambient } = inspectPortSurface();
  if (dead.length || retired.length || ambient.length) {
    console.error("check-port-surface: invalid application contracts:");
    for (const entry of dead) console.error(`  port clause: ${entry}`);
    for (const entry of retired)
      console.error(`  retired singleton API: ${entry}; capture an explicit feature instance`);
    for (const entry of ambient)
      console.error(`  ambient publication: ${entry}; construct it inside its owning instance`);
    process.exit(1);
  }
  console.log(
    `check-port-surface: ${files} sources covered; every port clause has a production consumer; no retired singleton APIs or ambient publications`,
  );
}
