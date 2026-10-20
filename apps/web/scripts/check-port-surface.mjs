#!/usr/bin/env node
import { relative, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { SymbolFlags } from "typescript/unstable/sync";
import * as ts from "typescript/unstable/ast";
import { visit, withSourceProject } from "./source-graph.mjs";

const isTest = (path) =>
  /\.(test|spec)\.(?:[cm]?ts|tsx)$/.test(path) ||
  path.startsWith("test/") ||
  path.includes("testkit");

function isValueReference(node) {
  if (
    node.parent?.name === node &&
    !ts.isPropertyAccessExpression(node.parent) &&
    !ts.isShorthandPropertyAssignment(node.parent)
  )
    return false;
  for (let parent = node.parent; parent; parent = parent.parent) {
    if (ts.isTypeNode(parent) || ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent))
      return false;
  }
  return true;
}

export function inspectPortSurface(root = process.cwd()) {
  return withSourceProject(root, (project, paths) => {
    const { program, checker } = project;
    const src = resolve(root, "src");
    const sources = paths
      .filter((path) => !isTest(relative(src, path)))
      .map((path) => program.getSourceFile(path));
    const clauses = new Map();
    const accessors = new Map();
    const used = new Set();
    const called = new Set();
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
        if (
          ts.isVariableStatement(node) &&
          node.modifiers?.some((modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword)
        ) {
          for (const declaration of node.declarationList.declarations) {
            const initial = declaration.initializer;
            if (
              initial &&
              ts.isPropertyAccessExpression(initial) &&
              initial.name.text === "get" &&
              declarations(checker.getSymbolAtLocation(initial.name)).some(
                (method) =>
                  ts.isInterfaceDeclaration(method.parent) &&
                  method.parent.name.text === "SingletonPort",
              )
            ) {
              accessors.set(key(declaration), `${path}  ${declaration.name.getText()}()`);
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
        if (ts.isCallExpression(node)) {
          record(checker.getSymbolAtLocation(node.expression), called);
        } else if (ts.isShorthandPropertyAssignment(node)) {
          record(checker.getShorthandAssignmentValueSymbol(node), called);
        } else if (ts.isIdentifier(node) && isValueReference(node)) {
          record(checker.getSymbolAtLocation(node), called);
        }
      });
    }
    return {
      files: paths.length,
      dead: [...clauses].filter(([id]) => !used.has(id)).map(([, label]) => label),
      bypasses: [...accessors].filter(([id]) => !called.has(id)).map(([, label]) => label),
    };
  });
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const { files, dead, bypasses } = inspectPortSurface();
  if (dead.length || bypasses.length) {
    console.error("check-port-surface: contracts without a production consumer:");
    for (const entry of dead) console.error(`  port clause: ${entry}`);
    for (const entry of bypasses) console.error(`  singleton accessor: ${entry}`);
    process.exit(1);
  }
  console.log(
    `check-port-surface: ${files} sources covered; every port clause and accessor has a production consumer`,
  );
}
