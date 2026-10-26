import console, { Console } from "node:console";
import { syncBuiltinESMExports } from "node:module";
import { Backend } from "./backend";
import { StdioTransport } from "./transport";

export async function startBackend(options: {
  load(): Promise<{ default: Backend }>;
}): Promise<void> {
  Object.assign(
    console,
    new Console({ stdout: process.stderr, stderr: process.stderr }),
  );
  globalThis.console = console;
  // Built-in named imports must observe the same redirected console before
  // the author and its dependencies are evaluated.
  syncBuiltinESMExports();
  let backend: Backend | undefined;
  let failure = false;
  let stopping = false;
  const shutdown = () => {
    stopping = true;
    void backend?.server.close().catch(() => {
      failure = true;
    });
  };
  const fatal = () => {
    failure = true;
    process.stderr.write("plugin: fatal backend failure\n");
    shutdown();
  };
  process.once("uncaughtException", fatal);
  process.once("unhandledRejection", fatal);
  process.once("SIGTERM", shutdown);
  process.once("SIGINT", shutdown);
  try {
    const loaded = await options.load();
    if (!(loaded.default instanceof Backend))
      throw new Error("plugin: module must export its Backend as default");
    backend = loaded.default;
    if (stopping) return;
    backend.publish();
    const closed = Promise.withResolvers<void>();
    backend.server.onclose = () => closed.resolve();
    backend.server.onerror = () => {
      failure = true;
    };
    await backend.server.connect(
      new StdioTransport(process.stdin, process.stdout),
    );
    await closed.promise;
  } catch {
    failure = true;
    process.stderr.write("plugin: backend startup or protocol failed\n");
  } finally {
    try {
      await backend?.close();
    } catch {
      failure = true;
      process.stderr.write("plugin: backend cleanup failed\n");
    }
    process.off("uncaughtException", fatal);
    process.off("unhandledRejection", fatal);
    process.off("SIGTERM", shutdown);
    process.off("SIGINT", shutdown);
    if (failure) process.exitCode = 1;
  }
}
