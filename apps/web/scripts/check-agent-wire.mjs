import { realpathSync } from "node:fs";
import { dirname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { SignatureKind, SymbolFlags } from "typescript/unstable/sync";
import { withSourceProject } from "./source-graph.mjs";

const sdk = dirname(
  dirname(realpathSync(fileURLToPath(import.meta.resolve("@flame/runtime-client")))),
);
export function inspectAgentWireTypes(root = process.cwd()) {
  return withSourceProject(root, (project, paths) => {
    const { program, checker } = project;
    const sourceRoot = resolve(root, "src");
    const violations = [];
    const memo = new Map();
    const declarations = (symbol) => {
      if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      return (symbol?.declarations ?? []).map((handle) => handle.resolve(project)).filter(Boolean);
    };
    function wireIn(type, seen = new Set()) {
      if (!type || seen.has(type.id)) return undefined;
      if (memo.has(type.id)) return memo.get(type.id);
      seen.add(type.id);
      const sources = [type.getSymbol(), type.getAliasSymbol()]
        .flatMap(declarations)
        .map((node) => node.getSourceFile().fileName);
      const direct = sources.find(
        (file) =>
          resolve(file).startsWith(sdk + sep) ||
          file.includes("/node_modules/@flame/runtime-client/"),
      );
      if (direct) return direct;
      const arguments_ = [
        ...type.getAliasTypeArguments(),
        ...(type.getBaseTypes() ?? []),
        ...(type.isTypeReference() ? checker.getTypeArguments(type) : []),
        ...(type.isUnionType() || type.isIntersectionType() ? type.getTypes() : []),
      ];
      if (type.isIndexedAccessType()) arguments_.push(type.getObjectType(), type.getIndexType());
      if (type.isConditionalType())
        arguments_.push(
          type.getCheckType(),
          type.getExtendsType(),
          type.getTrueType(),
          type.getFalseType(),
        );
      if (type.isIndexType() || type.isStringMappingType()) arguments_.push(type.getTarget());
      if (type.isSubstitutionType()) arguments_.push(type.getBaseType(), type.getConstraint());
      for (const argument of arguments_) {
        const found = wireIn(argument, seen);
        if (found) return found;
      }
      // Library containers are inspected through their arguments. Expanding DOM,
      // React and Array members would inspect unrelated global APIs as product types.
      if (sources.length && sources.every((file) => file.includes("/node_modules/")))
        return undefined;
      if (!type.isObjectType()) return undefined;
      for (const property of checker.getPropertiesOfType(type)) {
        const found = wireIn(checker.getTypeOfSymbol(property), seen);
        if (found) return found;
      }
      for (const kind of [SignatureKind.Call, SignatureKind.Construct]) {
        for (const signature of checker.getSignaturesOfType(type, kind)) {
          const found = wireIn(checker.getReturnTypeOfSignature(signature), seen);
          if (found) return found;
          for (const parameter of signature.getParameters()) {
            const nested = wireIn(checker.getTypeOfSymbol(parameter), seen);
            if (nested) return nested;
          }
        }
      }
      for (const index of checker.getIndexInfosOfType(type)) {
        const found = wireIn(index.type, seen);
        if (found) return found;
      }
      memo.set(type.id, undefined);
      return undefined;
    }
    for (const file of paths) {
      const name = relative(sourceRoot, file).split(sep).join("/");
      if (!name.startsWith("features/agent/public/") || /\.(test|spec)\./.test(name)) continue;
      const source = program.getSourceFile(file);
      const module = checker.getSymbolAtLocation(source);
      if (!module) continue;
      for (let symbol of checker.getExportsOfModule(module)) {
        if (symbol.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
        const type =
          symbol.flags & SymbolFlags.Type
            ? checker.getDeclaredTypeOfSymbol(symbol)
            : checker.getTypeOfSymbol(symbol);
        const found = wireIn(type);
        if (found) violations.push(`${name}: ${symbol.name} exposes ${relative(root, found)}`);
      }
    }
    return violations;
  });
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const violations = inspectAgentWireTypes();
  if (violations.length) throw new Error(violations.join("\n"));
  console.log("Agent public types resolve only to neutral product models.");
}
