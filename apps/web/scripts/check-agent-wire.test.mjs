import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { test } from "node:test";
import { inspectAgentWireTypes } from "./check-agent-wire.mjs";
import { assertSourceClosure } from "../../../tooling/source-graph.mjs";

function fixture(publicSource, inspect) {
  const root = mkdtempSync(join(tmpdir(), "flame-agent-wire-"));
  try {
    const files = {
      "tsconfig.json": JSON.stringify({
        compilerOptions: {
          module: "ESNext",
          moduleResolution: "Bundler",
          strict: true,
          types: [],
        },
        include: ["src"],
      }),
      "node_modules/@flame/runtime-client/package.json": JSON.stringify({
        name: "@flame/runtime-client",
        type: "module",
        exports: "./index.ts",
      }),
      "node_modules/@flame/runtime-client/index.ts":
        'export * from "./generated/wire.generated"; export interface Opening { runId: string; retry(): Opening }',
      "node_modules/@flame/runtime-client/generated/wire.generated.ts": [
        'export type ChangeStatus = "added" | "deleted" | "modified" | "moved";',
        'export type SafetyClass = "safe" | "write" | "exec" | "network";',
        "export interface RecordedChange { status: ChangeStatus; safetyClass: SafetyClass }",
      ].join("\n"),
      "src/features/agent/adapters/driver.ts":
        'import { type Opening } from "@flame/runtime-client"; export interface Driver { start(): Opening }',
      "src/features/agent/application/opening.ts":
        'import { type Driver } from "../adapters/driver"; export type Result = ReturnType<Driver["start"]>;',
      "src/features/agent/public/view.ts": publicSource,
    };
    for (const [name, content] of Object.entries(files)) {
      const file = join(root, name);
      mkdirSync(dirname(file), { recursive: true });
      writeFileSync(file, content);
    }
    inspect(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test("rejects a wire result hidden behind an inferred method, alias and mapped type", () => {
  fixture(
    'import { type Result } from "../application/opening"; export interface View { opening: Readonly<Result> }',
    (root) => {
      assert.match(inspectAgentWireTypes(root).join("\n"), /View exposes.*runtime-client/);
    },
  );
});

test("accepts a neutral projection while its adapter retains the wire handle", () => {
  fixture('export interface View { key: string; receipt: "unknown" | "notRecorded" }', (root) => {
    assert.deepEqual(inspectAgentWireTypes(root), []);
  });
});

test("accepts generated closed string enums in a neutral projection", () => {
  fixture(
    [
      'import { type ChangeStatus, type SafetyClass } from "@flame/runtime-client";',
      "export interface ChangeView { path: string; status: ChangeStatus; safetyClass?: SafetyClass }",
    ].join("\n"),
    (root) => {
      assert.deepEqual(inspectAgentWireTypes(root), []);
    },
  );
});

test("rejects a wire record even when it contains only enum values", () => {
  fixture(
    'import { type RecordedChange } from "@flame/runtime-client"; export interface ChangeView { change: RecordedChange }',
    (root) => {
      assert.match(inspectAgentWireTypes(root).join("\n"), /ChangeView exposes.*wire\.generated/);
    },
  );
});

test("rejects a union mixing an enum with a wire handle", () => {
  fixture(
    'import { type ChangeStatus, type Opening } from "@flame/runtime-client"; export interface MixedView { value: ChangeStatus | Opening }',
    (root) => {
      assert.match(inspectAgentWireTypes(root).join("\n"), /MixedView exposes.*runtime-client/);
    },
  );
});

test("rejects a deferred conditional branch containing an adapter result", () => {
  fixture(
    'import { type Result } from "../application/opening"; export type Conditional<T> = T extends true ? Result : string;',
    (root) => {
      assert.match(inspectAgentWireTypes(root).join("\n"), /Conditional exposes.*runtime-client/);
    },
  );
});

test("rejects a local interface that inherits wire authority", () => {
  fixture(
    'import { type Result } from "../application/opening"; export interface Inherited extends Result {}',
    (root) => {
      assert.match(inspectAgentWireTypes(root).join("\n"), /Inherited exposes.*runtime-client/);
    },
  );
});

test("accepts an independent JSON codec closure", () => {
  fixture("export interface View { key: string }", (root) => {
    const entry = join(root, "src/json.ts");
    writeFileSync(entry, 'export { encode } from "./codec";');
    writeFileSync(join(root, "src/codec.ts"), "export const encode = String;");
    assert.doesNotThrow(() =>
      assertSourceClosure(root, entry, new Set([join(root, "src/wire.ts")])),
    );
  });
});

test("rejects a JSON codec that imports Runtime authority transitively", () => {
  fixture("export interface View { key: string }", (root) => {
    const entry = join(root, "src/json.ts");
    const wire = join(root, "src/wire.ts");
    writeFileSync(entry, 'export { encode } from "./codec";');
    writeFileSync(join(root, "src/codec.ts"), 'export { encode } from "./wire";');
    writeFileSync(wire, "export const encode = String;");
    assert.throws(
      () => assertSourceClosure(root, entry, new Set([wire])),
      /source closure reaches forbidden entry: src\/wire\.ts/,
    );
  });
});
