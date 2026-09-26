import { createHash, randomUUID } from "node:crypto";
import {
  closeSync,
  existsSync,
  fsyncSync,
  linkSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { validateWire } from "@flame/runtime-contract/validate";
import type {
  CancelRunRequest,
  CreateSessionRequest,
  ResumeRunRequest,
  StartRunRequest,
} from "@flame/runtime-contract/wire";

export type Command =
  | { kind: "createSession"; params: CreateSessionRequest }
  | { kind: "start"; params: StartRunRequest }
  | { kind: "resume"; params: ResumeRunRequest }
  | { kind: "cancel"; params: CancelRunRequest };

export interface PendingCommand {
  id: string;
  namespace: string;
  expiresAt: number;
  command: Command;
}

const COMMAND_SHAPES = {
  createSession: "CreateSessionRequest",
  start: "StartRunRequest",
  resume: "ResumeRunRequest",
  cancel: "CancelRunRequest",
} as const;
const COMMAND_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function decode(text: string, id: string): PendingCommand {
  const value: unknown = JSON.parse(text);
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("invalid IDE command storage");
  const pending = value as Partial<PendingCommand>;
  if (
    Object.keys(pending).length !== 4 ||
    pending.id !== id ||
    typeof pending.namespace !== "string" ||
    !pending.namespace ||
    !Number.isSafeInteger(pending.expiresAt) ||
    !pending.command
  ) {
    throw new Error("invalid pending IDE command");
  }
  const kind = pending.command.kind;
  if (
    !Object.hasOwn(COMMAND_SHAPES, kind) ||
    validateWire(COMMAND_SHAPES[kind], pending.command.params).length > 0
  ) {
    throw new Error("invalid pending IDE command parameters");
  }
  return pending as PendingCommand;
}

export function authorizeCommand(pending: PendingCommand, namespace: string): void {
  if (pending.namespace !== namespace)
    throw new Error(
      "unresolved IDE command belongs to a different Runtime store; reconnect to the original store before retrying",
    );
  if (Date.now() >= pending.expiresAt)
    throw new Error(
      "unresolved command exceeded the Runtime replay retention; inspect the original Session before sending another command",
    );
}

export class CommandStore {
  readonly #directory: string;

  constructor(directory: string, endpoint: string) {
    this.#directory = join(directory, createHash("sha256").update(endpoint).digest("hex"));
    mkdirSync(this.#directory, { recursive: true, mode: 0o700 });
  }

  list(): PendingCommand[] {
    return readdirSync(this.#directory)
      .filter((name) => name.endsWith(".json"))
      .flatMap((name) => {
        const pending = this.read(name.slice(0, -5));
        return pending ? [pending] : [];
      });
  }

  read(id: string): PendingCommand | undefined {
    const path = this.#path(id);
    try {
      return decode(readFileSync(path, "utf8"), id);
    } catch (error) {
      if (hasCode(error, "ENOENT")) return undefined;
      throw error;
    }
  }

  prepare(namespace: string, retentionSeconds: number, command: Command): PendingCommand {
    const pending: PendingCommand = {
      id: randomUUID(),
      namespace,
      expiresAt: Date.now() + retentionSeconds * 1_000,
      command: structuredClone(command),
    };
    const path = this.#path(pending.id);
    const temporary = `${path}.tmp`;
    let descriptor: number | undefined;
    try {
      descriptor = openSync(temporary, "wx", 0o600);
      writeFileSync(descriptor, JSON.stringify(pending));
      fsyncSync(descriptor);
      closeSync(descriptor);
      descriptor = undefined;
      // Publication is atomic and exclusive; another extension host never reads partial input.
      linkSync(temporary, path);
      if (process.platform !== "win32") {
        descriptor = openSync(this.#directory, "r");
        fsyncSync(descriptor);
      }
    } finally {
      if (descriptor !== undefined) closeSync(descriptor);
      if (existsSync(temporary)) unlinkSync(temporary);
    }
    return pending;
  }

  settle(id: string): void {
    try {
      unlinkSync(this.#path(id));
    } catch (error) {
      if (!hasCode(error, "ENOENT")) throw error;
    }
  }

  #path(id: string): string {
    if (!COMMAND_ID.test(id)) throw new Error("invalid IDE command identity");
    return join(this.#directory, `${id}.json`);
  }
}

function hasCode(error: unknown, code: string): boolean {
  return error instanceof Error && "code" in error && error.code === code;
}
