import console from "node:console";
import { syncBuiltinESMExports } from "node:module";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Backend } from "./backend";
import { startBackend } from "./node";

const ownedEvents = new Set([
  "uncaughtException",
  "unhandledRejection",
  "SIGTERM",
  "SIGINT",
]);
const listeners = new Map<string | symbol, (...args: unknown[]) => void>();
let stderr: string[];
let stdout: string[];
let exitCode: typeof process.exitCode;
let globalConsole: typeof globalThis.console;
let consoleProperties: PropertyDescriptorMap;

beforeEach(() => {
  stderr = [];
  stdout = [];
  exitCode = process.exitCode;
  process.exitCode = undefined;
  globalConsole = globalThis.console;
  consoleProperties = Object.getOwnPropertyDescriptors(console);
  const once = process.once.bind(process);
  const off = process.off.bind(process);
  vi.spyOn(process, "once").mockImplementation((event, listener) => {
    if (typeof event === "string" && ownedEvents.has(event)) {
      listeners.set(event, listener);
      return process;
    }
    return once(event, listener);
  });
  vi.spyOn(process, "off").mockImplementation((event, listener) => {
    if (listeners.get(event) === listener) {
      listeners.delete(event);
      return process;
    }
    return off(event, listener);
  });
  vi.spyOn(process.stderr, "write").mockImplementation((chunk) => {
    stderr.push(String(chunk));
    return true;
  });
  vi.spyOn(process.stdout, "write").mockImplementation((chunk) => {
    stdout.push(String(chunk));
    return true;
  });
});

afterEach(() => {
  vi.restoreAllMocks();
  Object.defineProperties(console, consoleProperties);
  globalThis.console = globalConsole;
  syncBuiltinESMExports();
  process.exitCode = exitCode;
  listeners.clear();
});

function backend() {
  const instance = new Backend({ name: "fixture.bootstrap", version: "1" });
  const serve = vi.spyOn(instance, "serve").mockResolvedValue(undefined);
  const close = vi.spyOn(instance, "close").mockResolvedValue(undefined);
  return { instance, serve, close };
}
function expectFailure(phase: string) {
  expect(process.exitCode).toBe(1);
  expect(stderr.join("")).toContain(`plugin: backend ${phase} failed`);
  expect(stdout).toEqual([]);
  expect(listeners.size).toBe(0);
}

it("locates an import failure without logging its message or rejected payload", async () => {
  const failure = new SyntaxError("private author source", {
    cause: Object.assign(new Error("credential=private"), {
      code: "ERR_MODULE_NOT_FOUND",
      request: { authorization: "private" },
    }),
  });
  await startBackend({
    load: async () => {
      throw failure;
    },
  });
  expectFailure("startup");
  expect(stderr.join("")).toContain("error: SyntaxError");
  expect(stderr.join("")).toContain("caused by: Error [ERR_MODULE_NOT_FOUND]");
  expect(stderr.join("")).not.toContain("private");
});

it("retains service aggregate causes and delegates cleanup to Backend", async () => {
  const fixture = backend();
  fixture.serve.mockRejectedValue(
    new AggregateError([new SyntaxError("private frame")], "private service"),
  );
  await startBackend({ load: async () => ({ default: fixture.instance }) });
  expectFailure("service");
  expect(stderr.join("")).toContain("aggregate[0]: SyntaxError");
  expect(fixture.close).toHaveBeenCalled();
});

it("identifies a cleanup failure without turning the process exit into success", async () => {
  const fixture = backend();
  fixture.close.mockRejectedValue(
    new AggregateError([new TypeError("private resource")], "cleanup failed"),
  );
  await startBackend({ load: async () => ({ default: fixture.instance }) });
  expectFailure("cleanup");
  expect(stderr.join("")).toContain("aggregate[0]: TypeError");
});

it.each([
  ["uncaughtException", "uncaught exception"],
  ["unhandledRejection", "unhandled rejection"],
])(
  "reports %s and waits for the existing backend close",
  async (event, phase) => {
    const fixture = backend();
    const stopped = Promise.withResolvers<void>();
    fixture.close.mockImplementation(async () => stopped.resolve());
    fixture.serve.mockImplementation(() => {
      listeners.get(event)!(new RangeError("private payload"));
      return stopped.promise;
    });
    await startBackend({ load: async () => ({ default: fixture.instance }) });
    expectFailure(phase);
    expect(stderr.join("")).toContain("error: RangeError");
    expect(stderr.join("")).not.toContain("private payload");
    expect(fixture.close).toHaveBeenCalled();
  },
);

it("keeps normal shutdown successful and restores its process listeners", async () => {
  const fixture = backend();
  const stopped = Promise.withResolvers<void>();
  fixture.close.mockImplementation(async () => stopped.resolve());
  fixture.serve.mockImplementation(() => {
    listeners.get("SIGTERM")!();
    return stopped.promise;
  });
  await startBackend({ load: async () => ({ default: fixture.instance }) });
  expect(process.exitCode).toBeUndefined();
  expect(stderr).toEqual([]);
  expect(stdout).toEqual([]);
  expect(listeners.size).toBe(0);
});
