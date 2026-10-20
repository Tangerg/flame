import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { inspectPortSurface } from "./check-port-surface.mjs";

function inspect(source, tests = "") {
  const root = mkdtempSync(join(tmpdir(), "flame-port-surface-"));
  try {
    mkdirSync(join(root, "src"));
    writeFileSync(
      join(root, "tsconfig.json"),
      '{"compilerOptions":{"types":[]},"include":["src"]}',
    );
    writeFileSync(join(root, "src/consumer.ts"), source);
    writeFileSync(join(root, "src/consumer.test.ts"), tests);
    return inspectPortSurface(root);
  } finally {
    rmSync(root, { force: true, recursive: true });
  }
}

test("an unrelated method with the same name cannot satisfy a port contract", () => {
  const result = inspect(
    `
    export interface UsedPort { get(): void }
    export interface UnusedPort { get(): void }
    declare const used: UsedPort;
    used.get();
  `,
    `import type { UnusedPort } from './consumer'; declare const unused: UnusedPort; unused.get();`,
  );
  assert.equal(result.dead.length, 1);
  assert.match(result.dead[0], /UnusedPort.get/);
});

test("tracks destructuring, generics and literal property access by their owner", () => {
  const result = inspect(`
    interface ConsumerPort { read<T>(): T; inspect(): void; close(): void }
    declare const port: ConsumerPort;
    const { inspect: inspectValue } = port;
    inspectValue();
    port.read<string>();
    port['close']();
  `);
  assert.deepEqual(result.dead, []);
});

test("singleton accessors require an actual product call rather than import or test wiring", () => {
  const result = inspect(
    `
    interface SingletonPort<T> { get(): T }
    declare const port: SingletonPort<string>;
    export const live = port.get;
    export const unused = port.get;
    live();
  `,
    `import { unused } from './consumer'; unused();`,
  );
  assert.equal(result.bypasses.length, 1);
  assert.match(result.bypasses[0], /unused/);
});

test("ordinary get methods are not singleton accessors and callbacks are real consumers", () => {
  const result = inspect(`
    interface SingletonPort<T> { get(): T }
    declare const slot: SingletonPort<string>;
    export const live = slot.get;
    [1].map(live);
    declare const map: { get(key: string): string };
    export const lookup = map.get;
  `);
  assert.deepEqual(result.bypasses, []);
});

test("object shorthand consumes the accessor value rather than its property symbol", () => {
  const result = inspect(`
    interface SingletonPort<T> { get(): T }
    declare const slot: SingletonPort<string>;
    export const live = slot.get;
    export const unused = slot.get;
    export const dependencies = { live };
  `);
  assert.equal(result.bypasses.length, 1);
  assert.match(result.bypasses[0], /unused/);
});
