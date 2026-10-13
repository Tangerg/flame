#!/usr/bin/env node
import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { dirname, resolve, relative, sep } from "node:path";
import { API, SymbolFlags, TypeFlags } from "typescript/unstable/sync";
import * as ts from "typescript/unstable/ast";

const ROOT = process.cwd();
const METHODS_PATH = resolve(ROOT, "../../runtime/contract/typescript/client/methods.ts");
const SIDECAR_PATH = resolve(ROOT, "../../runtime/contract/typescript/client/sidecar.ts");
const WIRE_PATH = resolve(ROOT, "../../runtime/contract/typescript/wire.generated.ts");
const MANIFEST_PATH = resolve(ROOT, "../../runtime/contract/manifest.json");
const EVENT_POLICY_PATH = resolve(
  ROOT,
  "src/plugins/builtin/workspace/domain/eventInvalidation.ts",
);
const EVENT_ADAPTER_PATH = resolve(
  ROOT,
  "src/plugins/builtin/workspace/adapters/runtimeWorkspaceEvents.ts",
);
const TOOL_RESULT_PATH = resolve(ROOT, "src/plugins/sdk/toolResult.ts");
const RPC_ERRORS_PATH = resolve(ROOT, "src/lib/rpcErrors.ts");
const TSCONFIG_PATH = resolve(ROOT, "tsconfig.json");
const COMPONENTS_TSCONFIG_PATH = resolve(ROOT, "../../plugins/tsconfig.json");

const manifest = JSON.parse(readFileSync(MANIFEST_PATH, "utf8"));
const operations = new Set(manifest.methods.map((method) => method.name));
// Operations the terminal owns. Each still has a product consumer — the CLI —
// so the method is not orphaned; it simply has no reason to enter Desktop's SDK,
// and adding a UI solely for coverage would be the wrong trade.
//
//   tools.list / tools.invoke  — CLI /tools and /tool-invoke. Desktop presents
//     agent tool calls in the conversation and manages MCP in settings.
//   workspace.files.search     — CLI /grep. Desktop searches through the agent.
//   agentDocs.list             — CLI /agent-docs. Desktop reads AGENTS.md in the
//     file view like any other file.
const terminalOperations = new Set([
  "tools.list",
  "tools.invoke",
  "workspace.files.search",
  "agentDocs.list",
]);
// Desktop stopped reading `workspace.files.head`: a tool preview shows the recorded
// result, and the file view reads ranges through `workspace.files.read`. The method is
// Runtime's to retire; drop this entry in the same change that removes it there.
const awaitingRuntimeRemoval = new Set(["workspace.files.head"]);
const notDesktopOperations = new Set([...terminalOperations, ...awaitingRuntimeRemoval]);
const sidecarEndpoints = new Set(
  manifest.httpEndpoints
    .filter((endpoint) => endpoint.kind === "sidecar")
    .map((endpoint) => endpoint.name),
);
const runtimeTopics = new Set(manifest.runtimeTopics.map((topic) => topic.type));
// Every symbol a problem can arrive under: the JSON-RPC faults, the reasons a run channel
// ends, and the verdicts that ride a successful result.
const problemTypes = new Set([
  ...manifest.errors.types.map((error) => error.type),
  ...manifest.errors.runChannelTypes.map((problem) => problem.type),
  ...manifest.errors.inlineStatusTypes,
]);

// Every tool result the Runtime declares a schema for must have a reader in
// `plugins/sdk/toolResult.ts`, which is the one place those shapes are read through the
// generated types — so a renamed property is a compile error rather than an empty preview.
// The property names themselves are NOT checked here: an earlier attempt did, and passed a
// deliberate `hits` -> `matches` rename because `matches` appears in the tree for unrelated
// reasons. Generic names make that test look like coverage it does not have.
const schema = JSON.parse(
  readFileSync(resolve(ROOT, "../../runtime/contract/schema.json"), "utf8"),
);
const declaredResultTypes = new Set(
  manifest.toolResultPresentations.map((presentation) => presentation.schema.$ref.split("/").pop()),
);
for (const type of declaredResultTypes) {
  if (!schema.$defs[type]) throw new Error(`manifest names ${type}, absent from schema.json`);
}

const compiler = new API({ cwd: ROOT });
let compilerClosed = false;
function closeCompiler() {
  if (compilerClosed) return;
  compilerClosed = true;
  compiler.close();
}
process.once("exit", closeCompiler);
// Installed ESM has no static Desktop import. Its owning production project
// still supplies typed product callsites through the same public client.
const snapshot = compiler.updateSnapshot({
  openProjects: [TSCONFIG_PATH, COMPONENTS_TSCONFIG_PATH],
});
const project = snapshot.getProject(TSCONFIG_PATH);
if (!project) fail([`TypeScript did not load ${relative(ROOT, TSCONFIG_PATH)}`]);
const program = project.program;
let checker = project.checker;
const methodsSource = program.getSourceFile(METHODS_PATH);
if (!methodsSource) fail([`TypeScript did not load ${relative(ROOT, METHODS_PATH)}`]);
const sidecarSource = program.getSourceFile(SIDECAR_PATH);
if (!sidecarSource) fail([`TypeScript did not load ${relative(ROOT, SIDECAR_PATH)}`]);

const implementationMap = new Map();
const bindingFactories = new Set();
const bindingWrappers = new Set();
mapReturnedObject("bindWorkspace", [], true);
mapReturnedObject("createMethods", []);
const sidecarMethodMap = mappedSidecarMethods();

const consumerCalls = new Map();
const sidecarConsumerCalls = new Map();
const httpConsumerCalls = new Map();
const discardedResults = [];
let mappedErrorTypeCount = 0;
const componentsProject = snapshot.getProject(COMPONENTS_TSCONFIG_PATH);
if (!componentsProject) fail(["TypeScript did not load the installed components project"]);
const scannedSources = new Set();
for (const consumerProject of [project, componentsProject]) {
  checker = consumerProject.checker;
  for (const fileName of consumerProject.program.getSourceFileNames()) {
    const source = consumerProject.program.getSourceFile(fileName);
    if (!source) continue;
    const sourcePath = resolve(source.fileName);
    if (!isProductSource(sourcePath) || scannedSources.has(sourcePath)) continue;
    scannedSources.add(sourcePath);
    visit(source, (node) => {
      if (ts.isCallExpression(node)) {
        for (const endpoint of consumedHTTPEndpoints(node)) {
          const position = source.getLineAndCharacterOfPosition(node.getStart(source));
          const locations = httpConsumerCalls.get(endpoint) ?? new Set();
          locations.add(`${relative(ROOT, sourcePath)}:${position.line + 1}`);
          httpConsumerCalls.set(endpoint, locations);
        }
      }
      if (!ts.isCallExpression(node) || !ts.isPropertyAccessExpression(node.expression)) return;
      let symbol = checker.getSymbolAtLocation(node.expression.name);
      if (!symbol) return;
      if (symbol.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      for (const declaration of declarationsOf(symbol)) {
        const declarationPath = resolve(declaration.getSourceFile().fileName);
        let wrapper;
        let target;
        if (declarationPath === METHODS_PATH) {
          wrapper = wrapperPath(declaration, METHODS_PATH);
          target = consumerCalls;
        } else if (declarationPath === SIDECAR_PATH) {
          wrapper = wrapperPath(declaration, SIDECAR_PATH);
          target = sidecarConsumerCalls;
        } else {
          continue;
        }
        if (!wrapper || !target) continue;
        const position = source.getLineAndCharacterOfPosition(node.getStart(source));
        const location = `${relative(ROOT, sourcePath)}:${position.line + 1}`;
        const locations = target.get(wrapper) ?? new Set();
        locations.add(location);
        target.set(wrapper, locations);
        if (discardsNonVoidResult(node)) {
          discardedResults.push(`${wrapper} returns a value that is discarded at ${location}`);
        }
      }
    });
  }
}
checker = project.checker;

const errors = [];
errors.push(...discardedResults);
const operationConsumers = new Map([...operations].map((operation) => [operation, new Set()]));
for (const [wrapper, locations] of consumerCalls) {
  const mappedOperations = implementationMap.get(wrapper);
  if (!mappedOperations?.size) {
    if (bindingWrappers.has(wrapper)) continue;
    errors.push(
      `frontend calls Methods.${wrapper}, but its implementation could not be mapped to a Runtime operation (${[...locations].join(", ")})`,
    );
    continue;
  }
  for (const operation of mappedOperations) {
    const consumers = operationConsumers.get(operation);
    if (!consumers) {
      errors.push(
        `Methods.${wrapper} maps to ${operation}, which is absent from the Runtime manifest`,
      );
      continue;
    }
    for (const location of locations) consumers.add(location);
  }
}

const directlyConsumedOperations = new Set(
  [...operationConsumers]
    .filter(([, consumers]) => consumers.size > 0)
    .map(([operation]) => operation),
);
const httpProjectedOperations = creditHTTPOperationConsumers(
  manifest.httpEndpoints,
  manifest.methods,
  httpConsumerCalls,
  operationConsumers,
  errors,
);
const materializedOperations = creditMaterializedOperationConsumers(
  manifest.methods,
  operationConsumers,
  errors,
);

// Installed components use plugins.call. A Go-only operation must still
// have an actual production CLI or backend binding call; generated wrappers and inventories
// are insufficient evidence. Catalog names and Connection ports are parsed from Go
// source rather than listed as operation exemptions.
const headlessCalls = JSON.parse(
  execFileSync("go", ["run", "./runtime/cmd/apiconsumers", "."], {
    cwd: resolve(ROOT, "../.."),
    encoding: "utf8",
    maxBuffer: 1024 * 1024,
  }),
);
const headlessOperations = new Set();
for (const [operation, locations] of Object.entries(headlessCalls)) {
  const consumers = operationConsumers.get(operation);
  if (!consumers) {
    errors.push(`Go product calls non-manifest operation ${operation}`);
    continue;
  }
  if (consumers.size > 0 || notDesktopOperations.has(operation)) continue;
  for (const location of locations) consumers.add(`${location} (Go product binding)`);
  headlessOperations.add(operation);
}
checkOperationConsumers(operationConsumers, implementationMap, notDesktopOperations, errors);

checkSidecarConsumers(sidecarEndpoints, sidecarMethodMap, sidecarConsumerCalls, errors);

checkRuntimeTopics(errors);
checkToolResultReaders(errors);
checkMappedErrorTypes(errors);
if (errors.length > 0) fail(errors);

const callCount =
  [...consumerCalls.values()].reduce((total, locations) => total + locations.size, 0) +
  [...httpConsumerCalls.values()].reduce((total, locations) => total + locations.size, 0) +
  [...sidecarConsumerCalls.values()].reduce((total, locations) => total + locations.size, 0);
closeCompiler();
console.log(
  `check-backend-api-consumers: ${operations.size - notDesktopOperations.size}/${operations.size - notDesktopOperations.size} Runtime operation fact families have product coverage (${directlyConsumedOperations.size} direct graphical operations, ${headlessOperations.size} Go headless operations, ${materializedOperations.size} materialized through server composites, ${httpProjectedOperations.size} through consumed HTTP projections), ${sidecarEndpoints.size}/${sidecarEndpoints.size} HTTP sidecars, and ${runtimeTopics.size}/${runtimeTopics.size} event types have product consumers (${callCount} typed call sites); ${declaredResultTypes.size}/${declaredResultTypes.size} declared tool-result shapes have a typed reader; all ${mappedErrorTypeCount} error symbols the product writes copy for are ones a problem can arrive under (of ${problemTypes.size} declared)`,
);

function consumedHTTPEndpoints(call) {
  const imports = call.expression.kind === ts.SyntaxKind.ImportKeyword;
  let fetches = false;
  if (ts.isIdentifier(call.expression) && call.expression.text === "fetch") {
    const symbol = checker.getSymbolAtLocation(call.expression);
    fetches = declarationsOf(symbol).some((declaration) =>
      /[/\\]lib\.(dom|webworker)\.d\.ts$/.test(declaration.getSourceFile().fileName),
    );
  }
  if ((!imports && !fetches) || !call.arguments[0]) return new Set();
  const found = new Set();
  const scanned = new Set();
  function scan(expression) {
    const node = unwrapExpression(expression);
    if (
      ts.isPropertyAccessExpression(node) &&
      node.name.text === "path" &&
      ts.isPropertyAccessExpression(node.expression) &&
      ts.isIdentifier(node.expression.expression)
    ) {
      let symbol = checker.getSymbolAtLocation(node.expression.expression);
      if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      if (
        declarationsOf(symbol).some(
          (declaration) =>
            ts.isVariableDeclaration(declaration) &&
            ts.isIdentifier(declaration.name) &&
            declaration.name.text === "HTTP_ENDPOINTS" &&
            resolve(declaration.getSourceFile().fileName) === WIRE_PATH,
        )
      )
        found.add(node.expression.name.text);
      return;
    }
    if (ts.isIdentifier(node)) {
      let symbol = checker.getSymbolAtLocation(node);
      if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
      for (const declaration of declarationsOf(symbol)) {
        if (
          !ts.isVariableDeclaration(declaration) ||
          !declaration.initializer ||
          scanned.has(declaration)
        )
          continue;
        scanned.add(declaration);
        scan(declaration.initializer);
      }
    } else if (ts.isTemplateExpression(node)) {
      for (const span of node.templateSpans) scan(span.expression);
    } else if (ts.isBinaryExpression(node) && node.operatorToken.kind === ts.SyntaxKind.PlusToken) {
      scan(node.left);
      scan(node.right);
    } else if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      node.expression.name.text === "replace"
    ) {
      scan(node.expression.expression);
    }
  }
  scan(call.arguments[0]);
  return found;
}

function creditHTTPOperationConsumers(endpoints, methods, calls, consumers, targetErrors) {
  const credited = new Set();
  for (const endpoint of endpoints) {
    if (!endpoint.projects) continue;
    const method = methods.find((candidate) => candidate.name === endpoint.projects);
    if (endpoint.kind !== "resource" || method?.operation !== "query") {
      targetErrors.push(`${endpoint.name} projects an invalid Runtime query ${endpoint.projects}`);
      continue;
    }
    const locations = calls.get(endpoint.name);
    if (!locations?.size) continue;
    credited.add(endpoint.projects);
    for (const location of locations)
      consumers.get(endpoint.projects).add(`${location} (HTTP ${endpoint.name})`);
  }
  return credited;
}

function checkOperationConsumers(
  consumersByOperation,
  implementations,
  terminalOnly,
  targetErrors,
) {
  for (const operation of terminalOnly) {
    if (!consumersByOperation.has(operation)) {
      targetErrors.push(
        `${operation} is marked terminal-only but absent from the Runtime manifest`,
      );
    }
  }
  for (const [wrapper, operations] of implementations) {
    for (const operation of operations) {
      if (terminalOnly.has(operation)) {
        targetErrors.push(`Methods.${wrapper} exposes terminal-only diagnostic ${operation}`);
      }
    }
  }
  for (const [operation, consumers] of consumersByOperation) {
    if (!terminalOnly.has(operation) && consumers.size === 0) {
      targetErrors.push(
        `${operation} has no non-test frontend consumer (generated RPC wrappers and visual fixtures do not count)`,
      );
    }
  }
}

function creditMaterializedOperationConsumers(methods, consumersByOperation, targetErrors) {
  const credited = new Set();
  for (const method of methods) {
    if (!method.materializes?.length) continue;
    const compositeConsumers = consumersByOperation.get(method.name);
    if (!compositeConsumers?.size) continue;
    for (const materialized of method.materializes) {
      const target = consumersByOperation.get(materialized);
      if (!target) {
        targetErrors.push(`${method.name} materializes non-manifest operation ${materialized}`);
        continue;
      }
      if (target.size === 0) credited.add(materialized);
      for (const location of compositeConsumers) {
        target.add(`${location} (materialized by ${method.name})`);
      }
    }
  }
  return credited;
}

function mappedSidecarMethods() {
  let declaration;
  for (const statement of sidecarSource.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    declaration = statement.declarationList.declarations.find(
      (candidate) => ts.isIdentifier(candidate.name) && candidate.name.text === "SIDECAR_METHODS",
    );
    if (declaration) break;
  }
  const initializer = declaration?.initializer && unwrapExpression(declaration.initializer);
  if (!initializer || !ts.isObjectLiteralExpression(initializer)) {
    fail(["SIDECAR_METHODS must be an object literal in sidecar.ts"]);
  }
  const mapped = new Map();
  for (const property of initializer.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const endpoint = propertyName(property.name);
    const method = constantString(property.initializer);
    if (endpoint && method) mapped.set(endpoint, method);
  }
  return mapped;
}

function checkSidecarConsumers(expected, methods, calls, targetErrors) {
  for (const endpoint of expected) {
    const method = methods.get(endpoint);
    if (!method) {
      targetErrors.push(`${endpoint} has no typed SidecarClient method mapping`);
      continue;
    }
    const consumers = calls.get(method);
    if (!consumers?.size) {
      targetErrors.push(
        `${endpoint} has no non-test frontend consumer (the SidecarClient implementation and tests do not count)`,
      );
    }
  }
  for (const endpoint of methods.keys()) {
    if (!expected.has(endpoint)) {
      targetErrors.push(
        `SidecarClient maps ${endpoint}, which is absent from the Runtime manifest`,
      );
    }
  }
}

function mapReturnedObject(functionName, prefix, binding = false) {
  const declaration = methodsSource.statements.find(
    (statement) => ts.isFunctionDeclaration(statement) && statement.name?.text === functionName,
  );
  if (!declaration?.body) fail([`${functionName} implementation was not found in methods.ts`]);
  const returned = declaration.body.statements.find(
    (statement) =>
      ts.isReturnStatement(statement) && ts.isObjectLiteralExpression(statement.expression),
  );
  if (!returned || !ts.isReturnStatement(returned) || !returned.expression) {
    fail([`${functionName} must directly return its Methods object`]);
  }
  if (binding) bindingFactories.add(declaration);
  walkImplementationObject(returned.expression, prefix);
}

function walkImplementationObject(object, prefix) {
  for (const property of object.properties) {
    if (!ts.isPropertyAssignment(property)) continue;
    const name = propertyName(property.name);
    if (!name) continue;
    const path = [...prefix, name];
    if (ts.isObjectLiteralExpression(property.initializer)) {
      walkImplementationObject(property.initializer, path);
      continue;
    }
    const mapped = wireOperationsIn(property.initializer);
    const key = path.join(".");
    if (mapped.size === 0) {
      if (returnsMappedBinding(property.initializer)) bindingWrappers.add(key);
      continue;
    }
    const current = implementationMap.get(key) ?? new Set();
    for (const operation of mapped) current.add(operation);
    implementationMap.set(key, current);
  }
}

function returnsMappedBinding(initializer) {
  const functionBody =
    ts.isArrowFunction(initializer) || ts.isFunctionExpression(initializer)
      ? initializer.body
      : undefined;
  if (
    !functionBody ||
    !ts.isCallExpression(functionBody) ||
    !ts.isIdentifier(functionBody.expression)
  )
    return false;
  let symbol = checker.getSymbolAtLocation(functionBody.expression);
  if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
  return declarationsOf(symbol).some((declaration) => bindingFactories.has(declaration));
}

function wireOperationsIn(initializer) {
  const found = new Set();
  const scannedInitializers = new Set();

  const scanImplementation = (node) => {
    if (!ts.isIdentifier(node)) return;
    let symbol = checker.getSymbolAtLocation(node);
    if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
    for (const declaration of declarationsOf(symbol)) {
      if (
        !resolve(declaration.getSourceFile().fileName).startsWith(
          `${dirname(methodsSource.fileName)}${sep}`,
        )
      )
        continue;
      const implementation = ts.isVariableDeclaration(declaration)
        ? declaration.initializer
        : ts.isFunctionDeclaration(declaration)
          ? declaration.body
          : undefined;
      if (!implementation || scannedInitializers.has(declaration)) continue;
      scannedInitializers.add(declaration);
      scan(implementation);
    }
  };

  const scan = (node) => {
    if (ts.isFunctionDeclaration(node)) return;
    if (
      (ts.isVariableDeclaration(node) || ts.isPropertyAssignment(node)) &&
      node.initializer &&
      (ts.isArrowFunction(node.initializer) || ts.isFunctionExpression(node.initializer))
    )
      return;
    if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const operation = constantString(node.arguments[0]);
      if (operation && operations.has(operation)) found.add(operation);
    }
    if (ts.isCallExpression(node)) scanImplementation(node.expression);
    node.forEachChild(scan);
  };

  scanImplementation(initializer);
  scan(initializer);
  return found;
}

function constantString(expression) {
  const node = unwrapExpression(expression);
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) return node.text;
  if (!ts.isIdentifier(node)) return undefined;
  let symbol = checker.getSymbolAtLocation(node);
  if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
  for (const declaration of declarationsOf(symbol)) {
    if (ts.isVariableDeclaration(declaration) && declaration.initializer) {
      const value = constantString(declaration.initializer);
      if (value !== undefined) return value;
    }
  }
  return undefined;
}

function unwrapExpression(expression) {
  let node = expression;
  while (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isTypeAssertion(node)
  ) {
    node = node.expression;
  }
  return node;
}

function wrapperPath(declaration, declarationSourcePath) {
  const parts = [];
  let node = declaration;
  while (node && resolve(node.getSourceFile().fileName) === declarationSourcePath) {
    if (
      (ts.isPropertySignatureDeclaration(node) ||
        ts.isPropertyDeclaration(node) ||
        ts.isMethodSignatureDeclaration(node)) &&
      propertyName(node.name)
    ) {
      const name = propertyName(node.name);
      if (parts[0] !== name) parts.unshift(name);
    }
    if (ts.isInterfaceDeclaration(node)) break;
    node = node.parent;
  }
  return parts.join(".");
}

function discardsNonVoidResult(call) {
  let expression = call;
  let awaitedExpression;
  while (
    ts.isAwaitExpression(expression.parent) ||
    ts.isParenthesizedExpression(expression.parent) ||
    ts.isAsExpression(expression.parent) ||
    ts.isSatisfiesExpression(expression.parent) ||
    ts.isTypeAssertion(expression.parent) ||
    ts.isNonNullExpression(expression.parent) ||
    ts.isVoidExpression(expression.parent)
  ) {
    expression = expression.parent;
    if (ts.isAwaitExpression(expression)) awaitedExpression = expression;
  }
  if (!ts.isExpressionStatement(expression.parent)) return false;
  return hasMaterialResult(checker.getTypeAtLocation(awaitedExpression ?? call));
}

function hasMaterialResult(type) {
  if (!type) return true;
  if ((type.flags & (TypeFlags.Void | TypeFlags.Never)) !== 0) return false;
  if (type.isUnionType()) return type.getTypes().some(hasMaterialResult);
  if (!type.isTypeReference()) return true;
  const symbol = type.getSymbol();
  if (symbol?.name !== "Promise" && symbol?.name !== "PromiseLike") return true;
  const [resolved] = checker.getTypeArguments(type);
  return hasMaterialResult(resolved);
}

function propertyName(node) {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) {
    return node.text;
  }
  return undefined;
}

function isProductSource(sourcePath) {
  const sourceRoot = `${sep}src${sep}`;
  if (!sourcePath.includes(sourceRoot)) return false;
  if (sourcePath.includes(`${sourceRoot}rpc${sep}`)) return false;
  if (/\.(?:test|spec)\.[cm]?[jt]sx?$/.test(sourcePath)) return false;
  if (/\.generated\.[cm]?[jt]s$/.test(sourcePath)) return false;
  return true;
}

function checkToolResultReaders(targetErrors) {
  const source = program.getSourceFile(TOOL_RESULT_PATH);
  if (!source) {
    targetErrors.push("TypeScript did not load the tool-result reader");
    return;
  }
  const read = new Set();
  visit(source, (node) => {
    if (ts.isTypeReferenceNode(node) && ts.isIdentifier(node.typeName)) {
      read.add(node.typeName.text);
    }
  });
  for (const type of declaredResultTypes) {
    if (read.has(type)) continue;
    targetErrors.push(
      `${relative(ROOT, TOOL_RESULT_PATH)} has no reader for ${type}, declared by the Runtime for ` +
        manifest.toolResultPresentations
          .filter((p) => p.schema.$ref.endsWith(type))
          .map((p) => p.toolName)
          .join(", "),
    );
  }
}

function checkRuntimeTopics(targetErrors) {
  const policy = program.getSourceFile(EVENT_POLICY_PATH);
  const adapter = program.getSourceFile(EVENT_ADAPTER_PATH);
  if (!policy || !adapter) {
    targetErrors.push("TypeScript did not load the workspace event policy sources");
    return;
  }

  const ownedTypes = stringUnion(policy, "WorkspaceEventType");
  compareSets("WorkspaceEventType", runtimeTopics, ownedTypes, targetErrors);

  const handled = new Set();
  visit(policy, (node) => {
    if (ts.isCaseClause(node) && node.expression && ts.isStringLiteral(node.expression)) {
      if (runtimeTopics.has(node.expression.text)) handled.add(node.expression.text);
    }
  });
  compareSets("workspaceInvalidations switch", runtimeTopics, handled, targetErrors);

  const subscribed = stringArray(adapter, "SUBSCRIBED_TOPICS");
  const subscribable = new Set([...runtimeTopics].filter((topic) => topic !== "resync"));
  compareSets("SUBSCRIBED_TOPICS", subscribable, subscribed, targetErrors);
}

function stringUnion(source, name) {
  const declaration = source.statements.find(
    (statement) => ts.isTypeAliasDeclaration(statement) && statement.name.text === name,
  );
  if (!declaration || !ts.isUnionTypeNode(declaration.type)) return new Set();
  return new Set(
    declaration.type.types.flatMap((type) =>
      ts.isLiteralTypeNode(type) && ts.isStringLiteral(type.literal) ? [type.literal.text] : [],
    ),
  );
}

/**
 * Every symbol `rpcErrors` writes copy for is one a problem can actually arrive under.
 *
 * ONE DIRECTION only. The table is a curated subset and says so at each gap — the two replay
 * refusals are answered by reattaching rather than by a sentence, and the protocol faults have
 * their own paths. Demanding completeness would be demanding a sentence for `method_not_found`.
 *
 * What it catches is the other way round: a symbol nothing can send. `MAPPED_TYPES` is a plain
 * `string[]`, so a typo compiles, and `check:locales` agrees with it because the key exists in
 * every locale — the copy is simply never reached, and the error the reader does get falls
 * through to the generic line.
 */
function checkMappedErrorTypes(targetErrors) {
  const source = program.getSourceFile(RPC_ERRORS_PATH);
  if (!source) {
    targetErrors.push(`TypeScript did not load ${relative(ROOT, RPC_ERRORS_PATH)}`);
    return;
  }
  const mapped = stringArray(source, "MAPPED_TYPES");
  mappedErrorTypeCount = mapped.size;
  // Floor, not a target: a reader that found nothing agrees with any table.
  if (mapped.size === 0) {
    targetErrors.push("MAPPED_TYPES read as empty — this check is measuring nothing");
    return;
  }
  for (const type of mapped) {
    if (!problemTypes.has(type)) {
      targetErrors.push(`MAPPED_TYPES names ${type}, which no Runtime problem declares`);
    }
  }
}

function stringArray(source, name) {
  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || declaration.name.text !== name) continue;
      const initializer = declaration.initializer && unwrapExpression(declaration.initializer);
      if (!initializer || !ts.isArrayLiteralExpression(initializer)) return new Set();
      return new Set(
        initializer.elements.flatMap((element) =>
          ts.isStringLiteral(element) ? [element.text] : [],
        ),
      );
    }
  }
  return new Set();
}

function compareSets(label, expected, actual, targetErrors) {
  const missing = [...expected].filter((value) => !actual.has(value));
  const extra = [...actual].filter((value) => !expected.has(value));
  if (missing.length > 0) targetErrors.push(`${label} is missing: ${missing.join(", ")}`);
  if (extra.length > 0) targetErrors.push(`${label} has non-manifest values: ${extra.join(", ")}`);
}

function visit(node, callback) {
  callback(node);
  node.forEachChild((child) => visit(child, callback));
}

function declarationsOf(symbol) {
  return (symbol?.declarations ?? []).flatMap((handle) => {
    const declaration = handle.resolve(project);
    return declaration ? [declaration] : [];
  });
}

function fail(messages) {
  console.error("[check-backend-api-consumers] Failed:");
  for (const message of messages) console.error(`  - ${message}`);
  closeCompiler();
  process.exit(1);
}
