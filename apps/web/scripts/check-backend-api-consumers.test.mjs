import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const script = readFileSync(new URL("./check-backend-api-consumers.mjs", import.meta.url), "utf8");
const discardStart = script.indexOf("function discardsNonVoidResult(call)");
const discardEnd = script.indexOf("\nfunction propertyName", discardStart);
if (discardStart < 0 || discardEnd < 0) throw new Error("discarded-result helpers were not found");
const discardedResultHelpers = script.slice(discardStart, discardEnd);
const sidecarMatch =
  /function checkSidecarConsumers\(expected, methods, calls, targetErrors\) \{[\s\S]*?\n\}/.exec(
    script,
  );
if (!sidecarMatch) throw new Error("checkSidecarConsumers helper was not found");
const materializedMatch =
  /function creditMaterializedOperationConsumers\(methods, consumersByOperation, targetErrors\) \{[\s\S]*?\n\}/.exec(
    script,
  );
if (!materializedMatch)
  throw new Error("creditMaterializedOperationConsumers helper was not found");

function discardedResult(sourceText) {
  const dir = mkdtempSync(join(tmpdir(), "api-result-consumer-"));
  try {
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(join(dir, "probe.ts"), sourceText);
    writeFileSync(
      join(dir, "probe.mjs"),
      `import { API, TypeFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const fileName = ${JSON.stringify(join(dir, "probe.ts"))};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const snapshot = compiler.updateSnapshot({ openFiles: [fileName] });
  const project = snapshot.getDefaultProjectForFile(fileName);
  if (!project) throw new Error("TypeScript did not create an inferred project");
  const checker = project.checker;
  const source = project.program.getSourceFile(fileName);
  if (!source) throw new Error("TypeScript did not load the probe source");
  ${discardedResultHelpers}
  let call;
  function visit(node) { if (ts.isCallExpression(node) && node.expression.getText(source) === "saved") call = node; node.forEachChild(visit); }
  visit(source);
  console.log(discardsNonVoidResult(call));
} finally {
  compiler.close();
}
`,
    );
    const output = execFileSync(process.execPath, [join(dir, "probe.mjs")], {
      encoding: "utf8",
    }).trim();
    return output === "true";
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test("flags direct and explicit-void discards of a non-void SDK result", () => {
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<{ id: string }>; async function run() { await saved(); }",
    ),
    true,
  );
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<{ id: string }>; function run() { void saved(); }",
    ),
    true,
  );
});

test("allows a statement whose awaited result is void", () => {
  assert.equal(
    discardedResult(
      "declare function saved(): Promise<void>; async function run() { await saved(); }",
    ),
    false,
  );
});

test("fails a manifest sidecar with no product callsite", () => {
  const checkSidecarConsumers = Function(
    `return (${sidecarMatch[0].replace(/^function checkSidecarConsumers/, "function")})`,
  )();
  const errors = [];
  checkSidecarConsumers(
    new Set(["info", "liveness", "readiness"]),
    new Map([
      ["info", new Set(["info"])],
      ["liveness", new Set(["liveness"])],
      ["readiness", new Set(["readiness"])],
    ]),
    new Map([
      ["info", new Set(["runtimeServiceInspector.ts:1"])],
      ["liveness", new Set(["runtimeServiceInspector.ts:2"])],
    ]),
    errors,
  );
  assert.deepEqual(errors, [
    "readiness has no non-test frontend consumer (the SidecarClient implementation and tests do not count)",
  ]);
});

const sidecarImplementation = readFileSync(
  new URL("../../../packages/runtime-client/src/sidecar.ts", import.meta.url),
  "utf8",
);

function sidecarCoverage(sourceText, called = ["info", "liveness", "readiness"]) {
  const dir = mkdtempSync(join(tmpdir(), "api-sidecar-consumer-"));
  try {
    const fileName = join(dir, "sidecar.ts");
    const wire = join(dir, "wire.generated.ts");
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(
      fileName,
      sourceText.replaceAll('"@flame/runtime-client/wire"', '"./wire.generated"'),
    );
    writeFileSync(
      wire,
      "export const HTTP_ENDPOINTS = { info: {}, liveness: {}, readiness: {} };\n",
    );
    const helpers = [
      ["function mappedSidecarMethods()", "\nfunction mapReturnedObject"],
      ["function isHTTPEndpointCatalog(expression)", "\nfunction creditHTTPOperationConsumers"],
      ["function constantString(expression)", "\nfunction wrapperPath"],
      ["function propertyName(node)", "\nfunction isProductSource"],
      ["function declarationsOf(symbol)", "\nfunction fail"],
    ].map(([first, last]) => {
      const start = script.indexOf(first);
      const end = script.indexOf(last, start);
      assert.ok(start >= 0 && end > start, `${first} was not found`);
      return script.slice(start, end);
    });
    writeFileSync(
      join(dir, "probe.mjs"),
      `import { resolve } from "node:path";
import { API, SymbolFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const SIDECAR_PATH = ${JSON.stringify(fileName)};
  const WIRE_PATH = ${JSON.stringify(wire)};
  const snapshot = compiler.updateSnapshot({ openFiles: [SIDECAR_PATH] });
  const project = snapshot.getDefaultProjectForFile(SIDECAR_PATH);
  const checker = project.checker;
  const sidecarSource = project.program.getSourceFile(SIDECAR_PATH);
  class ScannerFailure extends Error {}
  function fail(messages) { throw new ScannerFailure(messages.join("; ")); }
  ${helpers.join("\n")}
  let methods = new Map();
  const errors = [];
  try {
    methods = mappedSidecarMethods();
    const calls = new Map(${JSON.stringify(called)}.map((method) => [method, new Set(["product.ts:1"])]));
    checkSidecarConsumers(new Set(["info", "liveness", "readiness"]), methods, calls, errors);
  } catch (error) {
    if (!(error instanceof ScannerFailure)) throw error;
    errors.push(error.message);
  }
  console.log(JSON.stringify({ methods: [...methods].map(([endpoint, members]) => [endpoint, [...members]]), errors }));
} finally { compiler.close(); }
`,
    );
    return JSON.parse(
      execFileSync(process.execPath, [join(dir, "probe.mjs")], { encoding: "utf8" }),
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test("maps the actual Sidecar implementation without a second endpoint table", () => {
  assert.deepEqual(sidecarCoverage(sidecarImplementation), {
    methods: [
      ["info", ["info"]],
      ["liveness", ["liveness"]],
      ["readiness", ["readiness"]],
    ],
    errors: [],
  });
});

test("follows renamed Sidecar methods, transport helpers and catalog imports", () => {
  const source = sidecarImplementation
    .replaceAll("getJson", "requestEndpoint")
    .replace("  HTTP_ENDPOINTS,", "  HTTP_ENDPOINTS as endpoints,")
    .replace("HTTP_ENDPOINTS[endpoint]", "endpoints[endpoint]")
    .replace("liveness:", "health:");
  assert.deepEqual(sidecarCoverage(source, ["info", "health", "readiness"]), {
    methods: [
      ["info", ["info"]],
      ["liveness", ["health"]],
      ["readiness", ["readiness"]],
    ],
    errors: [],
  });
});

test("checks actual Sidecar routes rather than matching member names", () => {
  const source = sidecarImplementation.replace(
    'getJson("liveness", signal)',
    'getJson("readiness", signal)',
  );
  assert.deepEqual(sidecarCoverage(source).errors, [
    "liveness has no typed SidecarClient method mapping",
  ]);
});

test("accepts any consumed Sidecar method that actually calls an endpoint", () => {
  const source = sidecarImplementation.replace(
    'liveness: (signal) => getJson("liveness", signal),',
    'liveness: (signal) => getJson("liveness", signal), health: (signal) => getJson("liveness", signal),',
  );
  assert.deepEqual(sidecarCoverage(source, ["info", "liveness", "readiness"]).errors, []);
  assert.deepEqual(sidecarCoverage(source, ["info", "health", "readiness"]).errors, []);
});

test("does not credit unused Sidecar callbacks or a local lookalike catalog", () => {
  const unused = sidecarImplementation.replace(
    'liveness: (signal) => getJson("liveness", signal),',
    'liveness: () => { const unused = () => getJson("liveness"); return Promise.resolve({}); },',
  );
  assert.deepEqual(sidecarCoverage(unused).errors, [
    "SidecarClient.liveness has no canonical HTTP endpoint call",
  ]);
  const local =
    sidecarImplementation.replace("  HTTP_ENDPOINTS,\n", "") +
    "\nconst HTTP_ENDPOINTS = { info: {}, liveness: {}, readiness: {} };\n";
  assert.deepEqual(sidecarCoverage(local).errors, [
    "SidecarClient.info has no canonical HTTP endpoint call",
  ]);
});

test("rejects dynamic and non-manifest Sidecar endpoint dispatch", () => {
  const dynamic = sidecarImplementation.replace(
    'getJson("info", signal)',
    "getJson(config.baseUrl, signal)",
  );
  assert.deepEqual(sidecarCoverage(dynamic).errors, [
    "SidecarClient.info has a dynamic HTTP endpoint",
  ]);
  const stale = sidecarImplementation.replace(
    'getJson("info", signal)',
    'getJson("retired", signal)',
  );
  assert.deepEqual(sidecarCoverage(stale).errors, [
    "info has no typed SidecarClient method mapping",
    "SidecarClient maps retired, which is absent from the Runtime manifest",
  ]);
});

test("credits query facts only when their server composite has a product callsite", () => {
  const creditMaterializedOperationConsumers = Function(
    `return (${materializedMatch[0].replace(
      /^function creditMaterializedOperationConsumers/,
      "function",
    )})`,
  )();
  const methods = [
    { name: "items.list" },
    { name: "runs.list" },
    { name: "sessions.snapshot", materializes: ["items.list", "runs.list"] },
  ];
  const consumers = new Map([
    ["items.list", new Set()],
    ["runs.list", new Set()],
    ["sessions.snapshot", new Set(["agentRuntimeGateway.ts:1"])],
  ]);
  const errors = [];

  const credited = creditMaterializedOperationConsumers(methods, consumers, errors);

  assert.deepEqual([...credited], ["items.list", "runs.list"]);
  assert.deepEqual(
    [...consumers.get("items.list")],
    ["agentRuntimeGateway.ts:1 (materialized by sessions.snapshot)"],
  );
  assert.deepEqual(errors, []);
});

const operationMatch = /function checkOperationConsumers\([^)]*\) \{[\s\S]*?\n\}/.exec(script);
if (!operationMatch) throw new Error("checkOperationConsumers helper was not found");
const checkOperationConsumers = Function(`return (${operationMatch[0]})`)();

test("requires product consumers for every operation outside terminal diagnostics", () => {
  const errors = [];
  checkOperationConsumers(
    new Map([
      ["tools.invoke", new Set()],
      ["runs.start", new Set()],
    ]),
    new Map(),
    new Set(["tools.invoke"]),
    errors,
  );
  assert.equal(errors.length, 1);
  assert.match(errors[0], /runs.start has no non-test frontend consumer/);
});

test("rejects terminal diagnostics in the Desktop SDK even without a caller", () => {
  const errors = [];
  checkOperationConsumers(
    new Map([["tools.invoke", new Set()]]),
    new Map([["tools.invoke", new Set(["tools.invoke"])]]),
    new Set(["tools.invoke"]),
    errors,
  );
  assert.equal(errors.length, 1);
  assert.match(errors[0], /exposes terminal-only diagnostic tools.invoke/);
});

test("rejects stale terminal diagnostic declarations", () => {
  const errors = [];
  checkOperationConsumers(new Map(), new Map(), new Set(["tools.invoke"]), errors);
  assert.equal(errors.length, 1);
  assert.match(errors[0], /absent from the Runtime manifest/);
});

function mappedOperations(sourceText, helperText, bindings = false) {
  const dir = mkdtempSync(join(tmpdir(), "api-operation-consumer-"));
  try {
    const fileName = join(dir, "probe.ts");
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(fileName, sourceText);
    if (helperText) writeFileSync(join(dir, "helper.ts"), helperText);
    const mappingStart = script.indexOf("function walkImplementationObject(object, prefix)");
    const mappingEnd = script.indexOf("\nfunction discardsNonVoidResult", mappingStart);
    const propertyStart = script.indexOf("function propertyName(node)");
    const propertyEnd = script.indexOf("\nfunction isProductSource", propertyStart);
    const declarationsStart = script.indexOf("function declarationsOf(symbol)");
    const declarationsEnd = script.indexOf("\nfunction fail", declarationsStart);
    assert.ok(mappingStart >= 0 && mappingEnd > mappingStart);
    assert.ok(declarationsStart >= 0 && declarationsEnd > declarationsStart);
    writeFileSync(
      join(dir, "probe.mjs"),
      `import { dirname, relative, resolve, sep } from "node:path";
import { API, SymbolFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const fileName = ${JSON.stringify(fileName)};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const snapshot = compiler.updateSnapshot({ openFiles: [fileName] });
  const project = snapshot.getDefaultProjectForFile(fileName);
  const checker = project.checker;
  const program = project.program;
  const methodsSource = program.getSourceFile(fileName);
  const ROOT = ${JSON.stringify(dir)};
  const METHODS_PATH = fileName;
  const implementationMap = new Map();
  const bindingFactories = new Set();
  const bindingWrappers = new Set();
  const bindingOwners = new Map();
  const operations = new Set(["runs.start", "runs.resume", "runs.getAcceptance"]);
  ${script.slice(mappingStart, mappingEnd)}
  ${script.slice(propertyStart, propertyEnd)}
  ${script.slice(declarationsStart, declarationsEnd)}
  let initializer;
  for (const statement of methodsSource.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name) && declaration.name.text === "start") initializer = declaration.initializer;
    }
  }
  if (${bindings}) {
    mapTypedSDKBindings();
    const found = new Set();
    function scan(node) {
      if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression)) {
        let symbol = checker.getSymbolAtLocation(node.expression.name);
        if (symbol?.flags & SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
        for (const declaration of declarationsOf(symbol)) {
          const path = typedBindingPath(declaration);
          for (const operation of implementationMap.get(path) ?? []) found.add(operation);
        }
      }
      node.forEachChild(scan);
    }
    scan(methodsSource);
    console.log(JSON.stringify({ projection: returnsMappedBinding(initializer), operations: [...found].sort() }));
  } else console.log(JSON.stringify([...wireOperationsIn(initializer)].sort()));
} finally { compiler.close(); }
`,
    );
    return JSON.parse(
      execFileSync(process.execPath, [join(dir, "probe.mjs")], { encoding: "utf8" }),
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test("follows a called SDK helper to its receipt read without crediting unused helpers", () => {
  assert.deepEqual(
    mappedOperations(`
      declare function perform(method: string): unknown;
      function read() { return perform("runs.getAcceptance"); }
      function openRun(method: string) {
        const unusedCallback = () => perform("runs.resume");
        function unusedLocal() { return perform("runs.resume"); }
        read(); return perform(method);
      }
      function unused() { return perform("runs.resume"); }
      const start = () => openRun("runs.start");
    `),
    ["runs.getAcceptance", "runs.start"],
  );
  assert.deepEqual(
    mappedOperations(`
      declare function perform(method: string): unknown;
      function read() { return perform("runs.getAcceptance"); }
      function openRun(method: string) { return perform(method); }
      const start = () => openRun("runs.start");
    `),
    ["runs.start"],
  );
});

test("bounds recursive SDK helpers and follows a variable implementation", () => {
  assert.deepEqual(
    mappedOperations(`
      declare function perform(method: string): unknown;
      function read() { recursive(); return perform("runs.getAcceptance"); }
      function recursive() { read(); }
      const openRun = (method: string) => { read(); return perform(method); };
      const start = () => openRun("runs.start");
    `),
    ["runs.getAcceptance", "runs.start"],
  );
});

test("does not count callable members of a returned SDK object as executed operations", () => {
  assert.deepEqual(
    mappedOperations(`
    declare function perform(method: string): unknown;
    function bind() { return { resume: () => perform("runs.resume") }; }
    function openRun(method: string) { bind(); return perform(method); }
    const start = () => openRun("runs.start");
  `),
    ["runs.start"],
  );
});

test("credits only actually called members of typed SDK capability projections", () => {
  const helper = `
    declare function perform(method: string): unknown;
    export interface Recovery {
      recover(): unknown;
      unused(): unknown;
    }
    export function bind(): Recovery {
      return {
        recover: () => perform("runs.getAcceptance"),
        unused: () => perform("runs.resume"),
      };
    }
  `;
  const projection = `
    import { bind } from "./helper";
    const start = () => [1].map(() => bind());
    const [command] = start();
  `;
  assert.deepEqual(mappedOperations(projection, helper, true), {
    projection: true,
    operations: [],
  });
  assert.deepEqual(mappedOperations(`${projection} command.recover();`, helper, true), {
    projection: true,
    operations: ["runs.getAcceptance"],
  });
  assert.deepEqual(mappedOperations(`${projection} command.unused();`, helper, true), {
    projection: true,
    operations: ["runs.resume"],
  });
});

test("does not credit an SDK helper without a product caller of its wrapper", () => {
  const errors = [];
  checkOperationConsumers(
    new Map([
      ["runs.start", new Set()],
      ["runs.getAcceptance", new Set()],
    ]),
    new Map([["runs.start", new Set(["runs.start", "runs.getAcceptance"])]]),
    new Set(),
    errors,
  );
  assert.deepEqual(errors, [
    "runs.start has no non-test frontend consumer (generated RPC wrappers and visual fixtures do not count)",
    "runs.getAcceptance has no non-test frontend consumer (generated RPC wrappers and visual fixtures do not count)",
  ]);
});

test("maps an imported SDK helper without crediting the returned binding's unused members", () => {
  assert.deepEqual(
    mappedOperations(
      `
    import { read } from "./helper";
    const start = () => read();
  `,
      `
    declare function perform(method: string): unknown;
    export function read() {
      const unused = { resume: () => perform("runs.resume") };
      return perform("runs.getAcceptance");
    }
  `,
    ),
    ["runs.getAcceptance"],
  );
});

function consumedHTTP(sourceText) {
  const dir = mkdtempSync(join(tmpdir(), "api-http-consumer-"));
  try {
    const fileName = join(dir, "probe.ts");
    const wire = join(dir, "wire.generated.ts");
    const modules = join(dir, "modules.ts");
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(fileName, sourceText);
    writeFileSync(
      wire,
      'export const HTTP_ENDPOINTS = { resources: { path: "/v2/resources/{lease}/*" } };',
    );
    writeFileSync(
      modules,
      "export class RealmModules { constructor(private open: () => Promise<{url: string}>) {} async evaluate() { const lease = await this.open(); return import(lease.url); } }",
    );
    const start = script.indexOf("function consumedHTTPEndpoints(call)");
    const end = script.indexOf("\nfunction creditHTTPOperationConsumers", start);
    const unwrapStart = script.indexOf("function unwrapExpression(expression)");
    const unwrapEnd = script.indexOf("\nfunction wrapperPath", unwrapStart);
    const declarationsStart = script.indexOf("function declarationsOf(symbol)");
    const declarationsEnd = script.indexOf("\nfunction fail", declarationsStart);
    assert.ok(start >= 0 && end > start);
    writeFileSync(
      join(dir, "probe.mjs"),
      `
import { resolve } from "node:path";
import { API, SymbolFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const snapshot = compiler.updateSnapshot({ openFiles: [${JSON.stringify(fileName)}] });
  const project = snapshot.getDefaultProjectForFile(${JSON.stringify(fileName)});
  const checker = project.checker;
  const source = project.program.getSourceFile(${JSON.stringify(fileName)});
  const WIRE_PATH = ${JSON.stringify(wire)};
  const MODULES_PATH = ${JSON.stringify(modules)};
  ${script.slice(start, end)}
  ${script.slice(unwrapStart, unwrapEnd)}
  ${script.slice(declarationsStart, declarationsEnd)}
  const found = new Set();
  function visit(node) {
    if (ts.isCallExpression(node)) for (const endpoint of consumedHTTPEndpoints(node)) found.add(endpoint);
    node.forEachChild(visit);
  }
  visit(source);
  console.log(JSON.stringify([...found]));
} finally { compiler.close(); }
`,
    );
    return JSON.parse(
      execFileSync(process.execPath, [join(dir, "probe.mjs")], { encoding: "utf8" }),
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

test("credits an imported resource URL only when consumed by module import", () => {
  assert.deepEqual(
    consumedHTTP(`
    import { HTTP_ENDPOINTS as endpoints } from "./wire.generated";
    const route = endpoints.resources.path.replace("{lease}", "token").replace("*", "index.js");
    const url = "https://runtime" + route;
    const loaded = import(url);
  `),
    ["resources"],
  );
  assert.deepEqual(
    consumedHTTP(`
    import { HTTP_ENDPOINTS } from "./wire.generated";
    const unused = HTTP_ENDPOINTS.resources.path;
    const loaded = import("https://unrelated/index.js");
  `),
    [],
  );
  assert.deepEqual(
    consumedHTTP(`
    const HTTP_ENDPOINTS = { resources: { path: "/invented" } };
    const loaded = import(HTTP_ENDPOINTS.resources.path);
  `),
    [],
  );
});

test("follows the actual Realm module opener only when its evaluation is consumed", () => {
  const source = `
    import { HTTP_ENDPOINTS } from "./wire.generated";
    import { RealmModules as Modules } from "./modules";
    class Loader {
      private modules = new Modules(() => this.open());
      async open() {
        const route = HTTP_ENDPOINTS.resources.path.replace("{lease}", "token");
        return { url: "https://runtime" + route };
      }
      async evaluate() { return this.modules.evaluate(); }
    }
    new Loader().evaluate();
  `;
  assert.deepEqual(consumedHTTP(source), ["resources"]);
  assert.deepEqual(
    consumedHTTP(source.replace("return this.modules.evaluate()", "return undefined")),
    [],
  );
  assert.deepEqual(
    consumedHTTP(
      source.replace(
        'import { RealmModules as Modules } from "./modules";',
        `
      class Modules {
        constructor(open: unknown) {}
        evaluate() { return import("https://unrelated/module.js"); }
      }
    `,
      ),
    ),
    [],
  );
  assert.deepEqual(
    consumedHTTP(
      source.replace(
        'return { url: "https://runtime" + route };',
        'const unused = () => ({ url: "https://runtime" + route }); return { url: "https://unrelated/module.js" };',
      ),
    ),
    [],
  );
});

test("credits only declared query projections with actual HTTP consumption", () => {
  const match = /function creditHTTPOperationConsumers\([^)]*\) \{[\s\S]*?\n\}/.exec(script);
  assert.ok(match);
  const credit = Function(`return (${match[0]})`)();
  const consumers = new Map([
    ["resource.read", new Set()],
    ["resource.write", new Set()],
  ]);
  const errors = [];
  const credited = credit(
    [
      { name: "resources", kind: "resource", projects: "resource.read" },
      { name: "unused", kind: "resource", projects: "resource.read" },
      { name: "invalid", kind: "resource", projects: "resource.write" },
    ],
    [
      { name: "resource.read", operation: "query" },
      { name: "resource.write", operation: "command" },
    ],
    new Map([["resources", new Set(["loader.ts:1"])]]),
    consumers,
    errors,
  );
  assert.deepEqual([...credited], ["resource.read"]);
  assert.deepEqual([...consumers.get("resource.read")], ["loader.ts:1 (HTTP resources)"]);
  assert.equal(consumers.get("resource.write").size, 0);
  assert.deepEqual(errors, ["invalid projects an invalid Runtime query resource.write"]);
});

test("credits RPC interface methods rather than command-handle methods in the same SDK source", () => {
  const dir = mkdtempSync(join(tmpdir(), "sdk-interface-consumer-"));
  try {
    const fileName = join(dir, "probe.ts");
    writeFileSync(join(dir, "package.json"), '{"type":"module"}');
    writeFileSync(
      fileName,
      `
      interface Methods { runs: { steer(): Promise<void>; getAcceptance(): Promise<void> }; }
      interface Command { retry(): Promise<void>; runs: { steer(): Promise<void> }; }
      declare const methods: Methods;
      declare const command: Command;
      methods.runs.steer(); methods.runs.getAcceptance(); command.retry(); command.runs.steer();
    `,
    );
    const start = script.indexOf("function wrapperPath(");
    const end = script.indexOf("\nfunction discardsNonVoidResult", start);
    const declarationStart = script.indexOf("function declarationsOf(symbol)");
    const declarationEnd = script.indexOf("\nfunction fail", declarationStart);
    assert.ok(start >= 0 && end > start);
    writeFileSync(
      join(dir, "probe.mjs"),
      `
      import { API } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
      import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
      import { resolve } from "node:path";
      const compiler = new API({ cwd: ${JSON.stringify(dir)} });
      try {
        const fileName = ${JSON.stringify(fileName)};
        const snapshot = compiler.updateSnapshot({ openFiles: [fileName] });
        const project = snapshot.getDefaultProjectForFile(fileName);
        const program = project.program;
        const checker = project.checker;
        ${script.slice(start, end)}
        ${script.slice(declarationStart, declarationEnd)}
        function propertyName(node) { return node?.text; }
        const paths = [];
        function visit(node) {
          if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression)) {
            const declaration = declarationsOf(checker.getSymbolAtLocation(node.expression.name))[0];
            paths.push(wrapperPath(declaration, fileName, new Set(["Methods"])));
          }
          node.forEachChild(visit);
        }
        visit(program.getSourceFile(fileName));
        console.log(JSON.stringify(paths));
      } finally { compiler.close(); }
    `,
    );
    assert.deepEqual(
      JSON.parse(execFileSync(process.execPath, [join(dir, "probe.mjs")], { encoding: "utf8" })),
      ["runs.steer", "runs.getAcceptance", "", ""],
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
