import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import { isAbsolute } from "node:path";
import { performance } from "node:perf_hooks";
import { TextDecoder } from "node:util";
import { parseJSON, stringifyJSON } from "@qimu/sdk/json";
import type { Transport } from "@modelcontextprotocol/sdk/shared/transport.js";
import {
  JSONRPCMessageSchema,
  type JSONRPCMessage,
} from "@modelcontextprotocol/sdk/types.js";
import { LIMITS } from "../contracts/limits";
import {
  EngineOwnership,
  processExists,
  type NativeTurn,
} from "./engineOwnership";
import { NativeLifecycle, type NativeConfig } from "./nativeLifecycle";

export interface EngineConfig {
  command: string;
  args: string[];
  env: Record<string, string>;
  ownershipPath: string;
  cwd?: string;
  native?: NativeConfig;
}

const EXIT_WAIT_MS = 2_000;

function environment(config: EngineConfig): Record<string, string> {
  if (
    !isAbsolute(config.command) ||
    !isAbsolute(config.ownershipPath) ||
    config.ownershipPath.includes("\0") ||
    config.command.includes("\0") ||
    !Array.isArray(config.args) ||
    config.args.some(
      (value) => typeof value !== "string" || value.includes("\0"),
    ) ||
    (config.cwd !== undefined &&
      (!isAbsolute(config.cwd) || config.cwd.includes("\0")))
  )
    throw new Error(
      "computer: engine requires an absolute executable and valid argv",
    );
  const env: Record<string, string> = {};
  for (const [name, value] of Object.entries(config.env)) {
    if (
      !/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) ||
      typeof value !== "string" ||
      value.includes("\0")
    )
      throw new Error("computer: invalid engine environment");
    env[name] = value;
  }
  if (config.native) {
    let services: unknown;
    try {
      services = JSON.parse(env.NODE_REPL_TRUSTED_SERVICES ?? "null");
    } catch (cause) {
      throw new Error("computer: native trusted-service selection is invalid", {
        cause,
      });
    }
    if (
      !services ||
      typeof services !== "object" ||
      Array.isArray(services) ||
      (services as Record<string, unknown>).sky !== config.native.wrapper
    )
      throw new Error(
        "computer: native execution requires its owned lifecycle service",
      );
  }
  return env;
}

// The transport owns the child group and bounded framing; the MCP SDK owns the protocol.
export class EngineProcess implements Transport {
  onclose?: Transport["onclose"];
  onerror?: Transport["onerror"];
  onmessage?: Transport["onmessage"];
  readonly #config: EngineConfig;
  readonly #environment: Record<string, string>;
  readonly #decoder = new TextDecoder("utf-8", { fatal: true });
  readonly #exited = Promise.withResolvers<void>();
  readonly #stderrCategories = new Set<"permission" | "sandbox" | "other">();
  #stderrLine = "";
  #chunks: Buffer[] = [];
  #frameBytes = 0;
  #stderrSkipping = false;
  #child?: ChildProcessWithoutNullStreams;
  #ownership?: EngineOwnership;
  #native?: NativeLifecycle;
  #ownershipFailure?: unknown;
  #opening?: Promise<void>;
  #started = false;
  #closed = false;
  #closing?: Promise<void>;

  constructor(config: EngineConfig) {
    this.#environment = environment(config);
    this.#config = structuredClone(config);
  }

  get diagnostics(): readonly string[] {
    return [...this.#stderrCategories].sort();
  }

  start(): Promise<void> {
    if (this.#started || this.#closing || this.#closed)
      throw new Error("computer: engine transport cannot restart");
    this.#started = true;
    this.#opening = Promise.resolve().then(() => this.#launch());
    return this.#opening;
  }

  async #launch(): Promise<void> {
    if (this.#closing)
      throw new Error("computer: engine startup was cancelled");
    try {
      this.#ownership = await EngineOwnership.acquire(
        this.#config.ownershipPath,
      );
    } catch (cause) {
      this.#ownershipFailure = cause;
      throw cause;
    }
    if (this.#closing)
      throw new Error("computer: engine startup was cancelled");
    if (this.#config.native) {
      this.#native = new NativeLifecycle(
        this.#config.native,
        this.#ownership,
        this.#environment,
      );
      Object.assign(this.#environment, await this.#native.start());
    }
    if (this.#closing)
      throw new Error("computer: engine startup was cancelled");
    const child = spawn(this.#config.command, this.#config.args, {
      cwd: this.#config.cwd,
      env: this.#environment,
      stdio: ["pipe", "pipe", "pipe"],
      shell: false,
      detached: process.platform !== "win32",
      windowsHide: true,
    });
    this.#child = child;
    child.once("close", () => {
      this.#native?.disconnected();
      this.#exited.resolve();
      this.#notifyClose();
    });
    child.on("error", (cause) =>
      this.#failed("computer: engine process failed", cause),
    );
    child.stdin.on("error", (cause) =>
      this.#failed("computer: engine input failed", cause),
    );
    child.stdout.on("error", (cause) =>
      this.#failed("computer: engine output failed", cause),
    );
    child.stderr.on("error", (cause) =>
      this.#failed("computer: engine diagnostics failed", cause),
    );
    child.stdout.on("data", (bytes: Buffer) => {
      if (this.#closed || this.#closing) return;
      try {
        this.#read(bytes);
      } catch (cause) {
        this.#failed("computer: invalid or oversized engine frame", cause);
      }
    });
    child.stdout.on("end", () => {
      if (this.#frameBytes)
        this.#failed(
          "computer: incomplete engine frame",
          new Error("engine output ended before its frame"),
        );
    });
    child.stderr.setEncoding("utf8");
    child.stderr.on("data", (text: string) => this.#diagnostic(text));
    await new Promise<void>((resolve, reject) => {
      child.once("spawn", resolve);
      child.once("error", reject);
    });
    if (!child.pid) throw new Error("computer: engine has no process identity");
    await this.#ownership.commit(child.pid);
  }

  #read(bytes: Buffer): void {
    for (let start = 0; start < bytes.length;) {
      const end = bytes.indexOf(10, start);
      const fragment = bytes.subarray(start, end < 0 ? bytes.length : end);
      if (this.#frameBytes + fragment.length > LIMITS.maxResultBytes)
        throw new Error("computer: engine frame exceeds capacity");
      this.#chunks.push(fragment);
      this.#frameBytes += fragment.length;
      if (end < 0) return;
      const source = this.#decoder.decode(
        Buffer.concat(this.#chunks, this.#frameBytes),
      );
      this.#chunks = [];
      this.#frameBytes = 0;
      const message = parseJSON(source);
      JSONRPCMessageSchema.parse(message);
      this.onmessage?.(message as JSONRPCMessage);
      start = end + 1;
    }
  }

  #diagnostic(text: string): void {
    for (const part of text.split(/(?<=\n)/)) {
      if (!this.#stderrSkipping) {
        this.#stderrLine += part.slice(
          0,
          Math.max(0, 1024 - this.#stderrLine.length),
        );
        this.#stderrSkipping = this.#stderrLine.length === 1024;
      }
      if (part.endsWith("\n") || this.#stderrSkipping) {
        const line = this.#stderrLine.toLowerCase();
        this.#stderrCategories.add(
          line.includes("sandbox")
            ? "sandbox"
            : line.includes("permission") || line.includes("accessibility")
              ? "permission"
              : "other",
        );
        this.#stderrLine = "";
        if (part.endsWith("\n")) this.#stderrSkipping = false;
      }
    }
  }

  #failed(message: string, cause: unknown): void {
    if (this.#closed || this.#closing) return;
    this.onerror?.(new Error(message, { cause }));
    void this.close().catch((error: unknown) => {
      this.onerror?.(
        new Error("computer: engine process cleanup failed", { cause: error }),
      );
    });
  }

  #notifyClose(): void {
    if (this.#closed) return;
    this.#closed = true;
    this.#chunks = [];
    this.#frameBytes = 0;
    this.onclose?.();
  }

  async send(message: JSONRPCMessage): Promise<void> {
    const child = this.#child;
    if (!child || this.#closed || this.#closing)
      throw new Error("computer: engine transport is closed");
    const frame = stringifyJSON(message) + "\n";
    if (Buffer.byteLength(frame) > LIMITS.maxResultBytes)
      throw new Error("computer: engine request exceeds frame capacity");
    await new Promise<void>((resolve, reject) =>
      child.stdin.write(frame, (error) => (error ? reject(error) : resolve())),
    );
  }

  beginTurn(turn: NativeTurn): void {
    this.#native?.begin(turn);
  }

  endAdmission(turn: NativeTurn): void {
    this.#native?.endAdmission(turn);
  }

  async finishTurn(turn: NativeTurn): Promise<void> {
    await this.#native?.finish(turn);
  }

  async #wait(): Promise<boolean> {
    const until = performance.now() + EXIT_WAIT_MS;
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      const exited = await Promise.race([
        this.#exited.promise.then(() => true),
        new Promise<false>((resolve) => {
          timer = setTimeout(() => resolve(false), EXIT_WAIT_MS);
        }),
      ]);
      if (!exited) return false;
      clearTimeout(timer);
      const pid = this.#child?.pid;
      let observationFailure: Error | undefined;
      while (pid !== undefined) {
        try {
          if (!processExists(pid)) return true;
          observationFailure = undefined;
        } catch (cause) {
          if (
            !(cause instanceof Error) ||
            !(cause.cause instanceof Error) ||
            !("code" in cause.cause) ||
            cause.cause.code !== "EPERM"
          )
            throw cause;
          // A denied probe cannot establish exit. Keep its cause until the
          // original close deadline or a later definitive absence observation.
          observationFailure = cause;
        }
        const remaining = until - performance.now();
        if (remaining <= 0) {
          if (observationFailure) throw observationFailure;
          return false;
        }
        await new Promise<void>((resolve) => {
          timer = setTimeout(resolve, Math.min(25, remaining));
        });
      }
      return true;
    } finally {
      clearTimeout(timer);
    }
  }

  #signal(signal: NodeJS.Signals): void {
    const child = this.#child;
    if (!child?.pid) return;
    try {
      if (process.platform === "win32") {
        if (child.exitCode === null && child.signalCode === null)
          child.kill(signal);
      } else process.kill(-child.pid, signal);
    } catch (cause) {
      if (!(
        cause instanceof Error &&
        "code" in cause &&
        cause.code === "ESRCH"
      ))
        throw new Error("computer: engine process group could not be stopped", {
          cause,
        });
    }
  }

  close(): Promise<void> {
    this.#closing ??= Promise.resolve().then(async () => {
      await this.#opening?.catch(() => {});
      const child = this.#child;
      const nativeClosing = this.#native?.close();
      void nativeClosing?.catch(() => {});
      const failures: unknown[] = [];
      try {
        try {
          if (child) {
            child.stdin.end();
            let exited = await this.#wait();
            if (!exited) {
              this.#signal("SIGTERM");
              exited = await this.#wait();
            }
            if (!exited) {
              this.#signal("SIGKILL");
              exited = await this.#wait();
            }
            if (!exited)
              throw new Error("computer: engine process exit is unconfirmed");
          }
        } catch (cause) {
          failures.push(cause);
        }
        try {
          await nativeClosing;
        } catch (cause) {
          failures.push(cause);
        }
        if (this.#ownershipFailure) {
          failures.push(
            new Error("computer: earlier engine ownership remains unresolved", {
              cause: this.#ownershipFailure,
            }),
          );
        }
        if (failures.length)
          throw new AggregateError(
            failures,
            "computer: engine cleanup is unconfirmed",
          );
        await this.#ownership?.release(child?.pid);
      } finally {
        this.#notifyClose();
      }
    });
    return this.#closing;
  }
}
