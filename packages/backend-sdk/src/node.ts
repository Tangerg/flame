import console, { Console } from "node:console";
import { syncBuiltinESMExports } from "node:module";
import { Backend } from "./backend";
import { failureDiagnostic, type FailurePhase } from "./diagnostic";
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
  let phase: FailurePhase = "startup";
  const report = (stage: FailurePhase, cause: unknown) => {
    failure = true;
    process.stderr.write(failureDiagnostic(stage, cause));
  };
  const shutdown = () => {
    stopping = true;
    void backend?.close().catch(() => {
      failure = true;
    });
  };
  const uncaught = (cause: unknown) => {
    report("uncaught exception", cause);
    shutdown();
  };
  const rejected = (cause: unknown) => {
    report("unhandled rejection", cause);
    shutdown();
  };
  process.once("uncaughtException", uncaught);
  process.once("unhandledRejection", rejected);
  process.once("SIGTERM", shutdown);
  process.once("SIGINT", shutdown);
  try {
    const loaded = await options.load();
    if (!(loaded.default instanceof Backend))
      throw new Error("plugin: module must export its Backend as default");
    backend = loaded.default;
    if (stopping) return;
    phase = "service";
    await backend.serve(new StdioTransport(process.stdin, process.stdout));
  } catch (cause) {
    report(phase, cause);
  } finally {
    try {
      await backend?.close();
    } catch (cause) {
      report("cleanup", cause);
    }
    process.off("uncaughtException", uncaught);
    process.off("unhandledRejection", rejected);
    process.off("SIGTERM", shutdown);
    process.off("SIGINT", shutdown);
    if (failure) process.exitCode = 1;
  }
}
