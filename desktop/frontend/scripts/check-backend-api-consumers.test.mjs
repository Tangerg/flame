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
      ["info", "info"],
      ["liveness", "liveness"],
      ["readiness", "readiness"],
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

function mappedOperations(sourceText, helperText) {
  const dir = mkdtempSync(join(tmpdir(), "api-operation-consumer-"));
  try {
    const fileName = join(dir, "probe.ts");
    writeFileSync(join(dir, "package.json"), '{"type":"module"}\n');
    writeFileSync(fileName, sourceText);
    if (helperText) writeFileSync(join(dir, "helper.ts"), helperText);
    const mappingStart = script.indexOf("function wireOperationsIn(initializer)");
    const mappingEnd = script.indexOf("\nfunction wrapperPath", mappingStart);
    const declarationsStart = script.indexOf("function declarationsOf(symbol)");
    const declarationsEnd = script.indexOf("\nfunction fail", declarationsStart);
    assert.ok(mappingStart >= 0 && mappingEnd > mappingStart);
    assert.ok(declarationsStart >= 0 && declarationsEnd > declarationsStart);
    writeFileSync(
      join(dir, "probe.mjs"),
      `import { dirname, resolve, sep } from "node:path";
import { API, SymbolFlags } from ${JSON.stringify(import.meta.resolve("typescript/unstable/sync"))};
import * as ts from ${JSON.stringify(import.meta.resolve("typescript/unstable/ast"))};
const fileName = ${JSON.stringify(fileName)};
const compiler = new API({ cwd: ${JSON.stringify(dir)} });
try {
  const snapshot = compiler.updateSnapshot({ openFiles: [fileName] });
  const project = snapshot.getDefaultProjectForFile(fileName);
  const checker = project.checker;
  const methodsSource = project.program.getSourceFile(fileName);
  const operations = new Set(["runs.start", "runs.resume", "runs.getAcceptance"]);
  ${script.slice(mappingStart, mappingEnd)}
  ${script.slice(declarationsStart, declarationsEnd)}
  let initializer;
  for (const statement of methodsSource.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (ts.isIdentifier(declaration.name) && declaration.name.text === "start") initializer = declaration.initializer;
    }
  }
  console.log(JSON.stringify([...wireOperationsIn(initializer)].sort()));
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
