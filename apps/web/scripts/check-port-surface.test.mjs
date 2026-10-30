import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { inspectPortSurface } from "./check-port-surface.mjs";

function inspect(source, tests = "", support = {}) {
  const root = mkdtempSync(join(tmpdir(), "flame-port-surface-"));
  try {
    mkdirSync(join(root, "src"));
    writeFileSync(
      join(root, "tsconfig.json"),
      '{"compilerOptions":{"types":[]},"include":["src"]}',
    );
    writeFileSync(join(root, "src/consumer.ts"), source);
    writeFileSync(join(root, "src/consumer.test.ts"), tests);
    for (const [name, content] of Object.entries(support))
      writeFileSync(join(root, "src", name), content);
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

test("rejects a restored singleton service API even when every accessor has a consumer", () => {
  const result = inspect(`
    interface SingletonPort<T> { get(): T; configure(next: T): () => void }
    declare function createSingletonPort<T>(): SingletonPort<T>;
    const slot = createSingletonPort<string>();
    export const current = slot.get;
    export const configure = slot.configure;
    configure('live')();
    [1].map(current);
  `);
  assert.deepEqual(result.dead, []);
  assert.match(result.retired.join("\n"), /SingletonPort/);
  assert.match(result.retired.join("\n"), /createSingletonPort/);
});

test("rejects renamed imports and re-exports of retired service publication", () => {
  const result = inspect(`
    import { createSingletonPort as createService } from './services';
    export { SingletonPort as ServiceSlot } from './services';
    import * as slots from './lib/ports/singletonPort';
    export * from '@/lib/ports/singletonPort';
    export const service = createService();
    export const publication = slots;
  `);
  assert.match(result.retired.join("\n"), /createSingletonPort/);
  assert.match(result.retired.join("\n"), /SingletonPort/);
  assert.match(result.retired.join("\n"), /\.\/lib\/ports\/singletonPort/);
  assert.match(result.retired.join("\n"), /@\/lib\/ports\/singletonPort/);
});

test("rejects retired module references in type, dynamic and require imports", () => {
  const result = inspect(`
    export type Port = import('./types/singletonPort').Port;
    export const load = () => import('./dynamic/singletonPort.js');
    const publication = require('./commonjs/singletonPort.cjs');
    import legacy = require('./legacy/singletonPort');
  `);
  assert.match(result.retired.join("\n"), /types\/singletonPort/);
  assert.match(result.retired.join("\n"), /dynamic\/singletonPort\.js/);
  assert.match(result.retired.join("\n"), /commonjs\/singletonPort\.cjs/);
  assert.match(result.retired.join("\n"), /legacy\/singletonPort/);
});

test("accepts captured instance ports and lifecycle publication without ambient services", () => {
  const result = inspect(
    `
    interface ConsumerPort { read(): string }
    declare function createPublicationSlot<T>(): { publish(value: T): void; current(): T | null };
    export function createFeature(port: ConsumerPort) {
      const publication = createPublicationSlot<ConsumerPort>();
      publication.publish(port);
      const read = port.read;
      return { read, current: publication.current };
    }
    declare const map: { get(key: string): string };
    export const lookup = map.get;
    export class Feature {
      readonly publication = createPublicationSlot<ConsumerPort>();
    }
    // SingletonPort and createSingletonPort describe the retired implementation.
    export const message = 'do not import ./lib/ports/singletonPort';
  `,
    `declare function createSingletonPort(): void; createSingletonPort();`,
  );
  assert.deepEqual(result.dead, []);
  assert.deepEqual(result.retired, []);
  assert.deepEqual(result.ambient ?? [], []);
});

test("rejects module-scoped publication through renamed imports, re-exports and aliases", () => {
  const result = inspect(
    `
    import { createPublicationSlot as makeSlot } from './publication';
    import * as slots from './publication';
    import { createGeneration } from './exports';
    const renamed = makeSlot;
    export const first = renamed();
    export const second = slots.createPublicationSlot();
    export const third = createGeneration();
    export class FeatureService {
      static current() { return first.current(); }
    }
    export const read = () => FeatureService.current();
    `,
    "",
    {
      "publication.ts": `export declare function createPublicationSlot<T>(): { current(): T | null };`,
      "exports.ts": `export { createPublicationSlot as createGeneration } from './publication';`,
    },
  );
  assert.equal(result.ambient?.length ?? 0, 3);
});

test("rejects a static global publication while allowing a class scoped to an instance factory", () => {
  const result = inspect(`
    declare function createPublicationSlot<T>(): { current(): T | null };
    export class AmbientOwner {
      static readonly publication = createPublicationSlot<object>();
      static current() { return this.publication.current(); }
    }
    export function createFeature() {
      class OwnedGeneration {
        static readonly publication = createPublicationSlot<object>();
      }
      return OwnedGeneration;
    }
  `);
  assert.equal(result.ambient?.length ?? 0, 1);
});
