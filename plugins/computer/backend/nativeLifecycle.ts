import { spawn, type ChildProcess } from "node:child_process";
import { randomBytes } from "node:crypto";
import { chmod, mkdtemp, rm } from "node:fs/promises";
import { createServer, type Server, type Socket } from "node:net";
import { tmpdir } from "node:os";
import { isAbsolute, join } from "node:path";
import { TextDecoder } from "node:util";
import {
  EngineOwnership,
  sameNativeTurn,
  type NativeTurn,
} from "./engineOwnership";

export interface NativeConfig {
  service: string;
  client: string;
  helper: string;
  wrapper: string;
}

interface ActiveNativeTurn {
  turn: NativeTurn;
  phase: "execution" | "cleanup";
  admitted?: Promise<void>;
  confirmation?: Promise<void>;
  helper?: Promise<void>;
  acknowledged: boolean;
  completed: ReturnType<typeof Promise.withResolvers<void>>;
}

const NATIVE_CLEANUP_MS = 30_000;
const HELPER_MS = 10_000;
const MAX_FRAME_BYTES = 8192;

function request(value: unknown): {
  token: string;
  method: "admit" | "ended" | "failed";
  turn: NativeTurn;
} {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !==
      "callId,method,sessionId,token,turnId"
  )
    throw new Error("computer: invalid native lifecycle message");
  const record = value as Record<string, unknown>;
  if (
    !["admit", "ended", "failed"].includes(String(record.method)) ||
    [record.token, record.sessionId, record.turnId, record.callId].some(
      (part) => typeof part !== "string" || !part || part.length > 1024,
    )
  )
    throw new Error("computer: invalid native lifecycle identity");
  return {
    token: record.token as string,
    method: record.method as "admit" | "ended" | "failed",
    turn: {
      sessionId: record.sessionId as string,
      turnId: record.turnId as string,
      callId: record.callId as string,
    },
  };
}

// Only the installed engine's trusted Sky worker can attest its native IPC acknowledgement.
export class NativeLifecycle {
  readonly #config: NativeConfig;
  readonly #ownership: EngineOwnership;
  readonly #environment: Record<string, string>;
  readonly #token = randomBytes(32).toString("hex");
  readonly #sockets = new Set<Socket>();
  readonly #requests = new Set<Promise<void>>();
  readonly #helpers = new Set<ChildProcess>();
  #directory?: string;
  #server?: Server;
  #active?: ActiveNativeTurn;
  #closing?: Promise<void>;

  constructor(
    config: NativeConfig,
    ownership: EngineOwnership,
    environment: Record<string, string>,
  ) {
    if (
      Object.values(config).some(
        (path) => !isAbsolute(path) || path.includes("\0"),
      )
    )
      throw new Error(
        "computer: native lifecycle requires selected absolute resources",
      );
    this.#config = { ...config };
    this.#ownership = ownership;
    this.#environment = { ...environment };
  }

  async start(): Promise<Record<string, string>> {
    if (this.#server || this.#closing)
      throw new Error("computer: native lifecycle cannot restart");
    this.#directory = await mkdtemp(
      join(process.platform === "darwin" ? "/tmp" : tmpdir(), "flame-cua-"),
    );
    await chmod(this.#directory, 0o700);
    const path = join(this.#directory, "native.sock");
    if (Buffer.byteLength(path) > 103)
      throw new Error("computer: native lifecycle socket path is too long");
    const server = createServer((socket) => this.#accept(socket));
    this.#server = server;
    await new Promise<void>((resolve, reject) => {
      const fail = (cause: Error) => {
        server.off("listening", ready);
        reject(cause);
      };
      const ready = () => {
        server.off("error", fail);
        resolve();
      };
      server.once("error", fail);
      server.once("listening", ready);
      server.listen(path);
    });
    await chmod(path, 0o600);
    server.on("error", (cause) => {
      this.#active?.completed.reject(
        new Error("computer: native lifecycle listener failed", { cause }),
      );
    });
    return {
      FLAME_COMPUTER_NATIVE_HOST: JSON.stringify({
        socket: path,
        token: this.#token,
        service: this.#config.service,
        client: this.#config.client,
      }),
    };
  }

  begin(turn: NativeTurn): void {
    if (!this.#server || this.#closing || this.#active)
      throw new Error(
        "computer: native lifecycle has no exclusive active turn",
      );
    const completed = Promise.withResolvers<void>();
    void completed.promise.catch(() => {});
    this.#active = {
      turn: {
        sessionId: turn.sessionId,
        turnId: turn.turnId,
        callId: turn.callId,
      },
      phase: "execution",
      acknowledged: false,
      completed,
    };
  }

  endAdmission(turn: NativeTurn): void {
    this.#current(turn).phase = "cleanup";
  }

  disconnected(): void {
    const active = this.#active;
    if (active) {
      active.phase = "cleanup";
      if (!active.acknowledged)
        active.completed.reject(
          new Error(
            "computer: native worker exited before cleanup acknowledgement",
          ),
        );
    }
  }

  async finish(turn: NativeTurn): Promise<void> {
    const active = this.#current(turn);
    active.phase = "cleanup";
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      if (active.admitted) {
        await active.admitted;
        await Promise.race([
          active.completed.promise,
          new Promise<never>((_, reject) => {
            timer = setTimeout(
              () =>
                reject(
                  new Error(
                    "computer: native cleanup acknowledgement timed out",
                  ),
                ),
              NATIVE_CLEANUP_MS,
            );
          }),
        ]);
      }
      this.#active = undefined;
    } finally {
      clearTimeout(timer);
    }
  }

  #current(turn: NativeTurn): ActiveNativeTurn {
    const active = this.#active;
    if (!active || !sameNativeTurn(active.turn, turn))
      throw new Error("computer: native lifecycle target is not current");
    return active;
  }

  #accept(socket: Socket): void {
    if (this.#closing || this.#sockets.size >= 8) {
      socket.destroy();
      return;
    }
    this.#sockets.add(socket);
    socket.once("close", () => this.#sockets.delete(socket));
    socket.on("error", () => {});
    socket.setTimeout(NATIVE_CLEANUP_MS, () => socket.destroy());
    let bytes = Buffer.alloc(0);
    const receive = (chunk: Buffer) => {
      if (bytes.length + chunk.length > MAX_FRAME_BYTES) {
        socket.destroy();
        return;
      }
      bytes = Buffer.concat([bytes, chunk]);
      const newline = bytes.indexOf(10);
      if (newline < 0) return;
      socket.off("data", receive);
      const pending = Promise.resolve()
        .then(async () => {
          const decoded = new TextDecoder("utf-8", { fatal: true }).decode(
            bytes.subarray(0, newline),
          );
          const message = request(JSON.parse(decoded));
          if (message.token !== this.#token || this.#closing)
            throw new Error(
              "computer: native lifecycle caller is not admitted",
            );
          await this.#route(message.method, message.turn);
          socket.end('{"accepted":true}\n');
        })
        .catch(() => {
          socket.end('{"accepted":false}\n');
        });
      this.#requests.add(pending);
      void pending.finally(() => this.#requests.delete(pending));
    };
    socket.on("data", receive);
  }

  async #route(
    method: "admit" | "ended" | "failed",
    turn: NativeTurn,
  ): Promise<void> {
    const active = this.#current(turn);
    if (method === "admit") {
      if (active.phase !== "execution")
        throw new Error("computer: native turn no longer accepts input");
      active.admitted ??= this.#ownership.admitNative(turn);
      await active.admitted;
      if (active.phase !== "execution" || this.#closing)
        throw new Error("computer: native turn ended during admission");
      return;
    }
    if (active.phase !== "cleanup" || !active.admitted)
      throw new Error("computer: native cleanup has no admitted turn");
    await active.admitted;
    if (method === "failed") {
      active.completed.reject(
        new Error("computer: native engine refused turn cleanup"),
      );
      return;
    }
    if (!active.confirmation) {
      active.confirmation = Promise.resolve().then(async () => {
        await this.#ownership.endNative(turn);
        active.acknowledged = true;
        await this.#runHelper(active);
      });
      void active.confirmation.then(
        active.completed.resolve,
        active.completed.reject,
      );
    }
    await active.confirmation;
  }

  #runHelper(active: ActiveNativeTurn): Promise<void> {
    active.helper ??= new Promise<void>((resolve, reject) => {
      const child = spawn(
        this.#config.helper,
        [
          "turn-ended",
          JSON.stringify({
            type: "agent-turn-complete",
            "thread-id": active.turn.sessionId,
            "turn-id": active.turn.turnId,
          }),
        ],
        {
          env: this.#environment,
          stdio: ["ignore", "ignore", "pipe"],
          shell: false,
        },
      );
      this.#helpers.add(child);
      child.stderr?.resume();
      let expired = false;
      const timer = setTimeout(() => {
        expired = true;
        child.kill("SIGKILL");
      }, HELPER_MS);
      const abandon = setTimeout(
        () => reject(new Error("computer: native helper exit is unconfirmed")),
        HELPER_MS + 2000,
      );
      child.once("error", (cause) => {
        clearTimeout(timer);
        clearTimeout(abandon);
        this.#helpers.delete(child);
        reject(
          new Error("computer: native cleanup helper could not start", {
            cause,
          }),
        );
      });
      child.once("close", (code) => {
        clearTimeout(timer);
        clearTimeout(abandon);
        this.#helpers.delete(child);
        if (code === 0 && !expired) resolve();
        else
          reject(new Error("computer: native cleanup helper did not complete"));
      });
    });
    return active.helper;
  }

  close(): Promise<void> {
    this.#closing ??= Promise.resolve().then(async () => {
      const active = this.#active;
      if (active) {
        active.phase = "cleanup";
        active.completed.reject(
          new Error("computer: native lifecycle was closed"),
        );
      }
      for (const socket of this.#sockets) socket.destroy();
      await Promise.allSettled(this.#requests);
      let failure: unknown;
      try {
        if (active?.admitted) {
          await active.admitted;
          await this.#runHelper(active);
          if (!active.acknowledged)
            throw new Error(
              "computer: native cleanup is unconfirmed; original IPC acknowledgement is missing",
            );
        }
      } catch (cause) {
        failure = cause;
      }
      for (const child of this.#helpers) child.kill("SIGKILL");
      if (this.#server)
        await new Promise<void>((resolve) =>
          this.#server!.close(() => resolve()),
        );
      if (this.#directory)
        await rm(this.#directory, { recursive: true, force: true });
      if (failure) throw failure;
    });
    return this.#closing;
  }
}
