import { DatabaseSync, type SQLOutputValue } from "node:sqlite";
import { isAbsolute } from "node:path";
import {
  Refusal,
  canonicalJSON,
  digestJSON,
  parseJSON,
  type Invocation,
  type ReceiptRequest,
} from "@qimu/node";
import { validCommand } from "@qimu/node/contract";
import type { ToolReceipt } from "@flame/backend-sdk";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { admitToolResult, projectToolOutput } from "./tool-result";

export interface ReceiptLimits {
  readonly maxRecords: number;
  readonly maxBytes: number;
  readonly maxResultBytes: number;
  readonly maxInputBytes: number;
}

export interface OriginalCommand {
  readonly id: string;
  readonly kind: "tool" | "command";
  readonly method: string;
  readonly input: string;
  readonly inputDigest: string;
  readonly invocation: Invocation;
  readonly admittedAt: string;
}

export type CommandReply =
  | { readonly value: unknown }
  | { readonly failure: { readonly code: string; readonly message: string } };

export type ReceiptRecord = OriginalCommand & {
  readonly dispatched: boolean;
} & (
    | { readonly state: "pending" }
    | {
        readonly state: "settled";
        readonly outcome: CallToolResult | CommandReply;
      }
  );

export interface ReceiptSummary {
  readonly id: string;
  readonly kind: "tool" | "command";
  readonly method: string;
  readonly admittedAt: string;
  readonly dispatched: boolean;
  readonly state: "pending" | "settled";
}

export interface ReceiptPage {
  readonly records: ReceiptSummary[];
  readonly nextCursor?: string;
}

export type ReceiptAdmission =
  | { readonly fresh: true }
  | { readonly fresh: false; readonly record: ReceiptRecord };

type Row = Record<string, SQLOutputValue>;

const schema =
  "CREATE TABLE commands (sequence INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('tool','command')), method TEXT NOT NULL, input TEXT NOT NULL CHECK(json_valid(input)), digest TEXT NOT NULL, invocation TEXT NOT NULL CHECK(json_valid(invocation)), admitted_at TEXT NOT NULL, dispatched INTEGER NOT NULL DEFAULT 0 CHECK(dispatched IN (0,1)), outcome TEXT CHECK(outcome IS NULL OR json_valid(outcome)))";

const columns =
  "id,kind,method,input,digest,invocation,admitted_at,dispatched,outcome";

function conflict(): never {
  throw new Refusal(
    "command_conflict",
    "original computer command identity binds different evidence",
  );
}

function object(value: unknown): asserts value is Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("computer: receipt evidence must be an object");
}

function invocation(value: unknown): asserts value is Invocation {
  object(value);
  object(value.target);
  if (
    Object.keys(value).some(
      (key) => !["id", "origin", "parent", "target"].includes(key),
    ) ||
    Object.keys(value.target).some(
      (key) => !["scope", "reference"].includes(key),
    ) ||
    !validCommand(value.id) ||
    /[\p{White_Space}\p{Cc}]/u.test(value.id) ||
    !["human", "model", "automatic"].includes(String(value.origin)) ||
    (value.parent !== undefined &&
      (!validCommand(value.parent) ||
        /[\p{White_Space}\p{Cc}]/u.test(value.parent) ||
        value.parent === value.id)) ||
    !validCommand(value.target.scope) ||
    /[\p{White_Space}\p{Cc}]/u.test(value.target.scope) ||
    typeof value.target.reference !== "string" ||
    !value.target.reference ||
    /[\p{Cc}\p{Surrogate}]/u.test(value.target.reference)
  )
    throw new Error("computer: trusted receipt invocation is required");
}

function admitted(
  original: OriginalCommand,
  limits: ReceiptLimits,
): { owner: string; bytes: number } {
  invocation(original.invocation);
  if (
    !validCommand(original.id) ||
    !validCommand(original.method) ||
    !["tool", "command"].includes(original.kind) ||
    typeof original.input !== "string" ||
    !/^[a-f0-9]{64}$/.test(original.inputDigest) ||
    typeof original.admittedAt !== "string" ||
    !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/.test(original.admittedAt) ||
    !Number.isFinite(Date.parse(original.admittedAt)) ||
    new Date(original.admittedAt).toISOString() !== original.admittedAt
  )
    throw new Error("computer: original command evidence is invalid");
  const owner = canonicalJSON(original.invocation);
  const bytes = [
    original.id,
    original.kind,
    original.method,
    original.input,
    original.inputDigest,
    owner,
    original.admittedAt,
  ].reduce((size, value) => size + Buffer.byteLength(value), 0);
  if (bytes > limits.maxInputBytes)
    throw new Refusal(
      "input_capacity",
      "computer input capacity exceeded by original command evidence",
    );
  const input = parseJSON(original.input);
  if (
    canonicalJSON(input) !== original.input ||
    digestJSON(input) !== original.inputDigest
  )
    throw new Error("computer: original input differs from its digest");
  return { owner, bytes };
}

function commandReply(value: unknown): asserts value is CommandReply {
  object(value);
  const keys = Object.keys(value);
  if (keys.length !== 1)
    throw new Error("computer: receipt requires one known command outcome");
  if (Object.hasOwn(value, "value")) return;
  if (!Object.hasOwn(value, "failure"))
    throw new Error("computer: receipt command outcome is invalid");
  object(value.failure);
  if (
    Object.keys(value.failure).length !== 2 ||
    typeof value.failure.code !== "string" ||
    !value.failure.code ||
    typeof value.failure.message !== "string"
  )
    throw new Error("computer: receipt refusal is invalid");
}

function summary(row: Row): ReceiptSummary {
  if (
    typeof row.id !== "string" ||
    !validCommand(row.id) ||
    (row.kind !== "tool" && row.kind !== "command") ||
    typeof row.method !== "string" ||
    !validCommand(row.method) ||
    typeof row.admitted_at !== "string" ||
    !Number.isFinite(Date.parse(row.admitted_at)) ||
    (row.dispatched !== 0n && row.dispatched !== 1n) ||
    (row.settled !== 0n && row.settled !== 1n)
  )
    throw new Error("computer: receipt header is corrupt");
  return {
    id: row.id,
    kind: row.kind,
    method: row.method,
    admittedAt: row.admitted_at,
    dispatched: row.dispatched === 1n,
    state: row.settled === 1n ? "settled" : "pending",
  };
}

export class Receipts {
  readonly #db: DatabaseSync;
  readonly #limits: ReceiptLimits;
  #closed = false;

  constructor(path: string, limits: ReceiptLimits) {
    if (
      !isAbsolute(path) ||
      ![
        limits.maxRecords,
        limits.maxBytes,
        limits.maxResultBytes,
        limits.maxInputBytes,
      ].every((value) => Number.isSafeInteger(value) && value > 0) ||
      limits.maxRecords < 2 ||
      limits.maxInputBytes >= limits.maxBytes - limits.maxResultBytes
    )
      throw new Error("computer: positive finite receipt budgets are required");
    this.#limits = Object.freeze({ ...limits });
    const db = new DatabaseSync(path, {
      allowExtension: false,
      enableDoubleQuotedStringLiterals: false,
      readBigInts: true,
      timeout: 0,
    });
    this.#db = db;
    try {
      db.exec(
        "PRAGMA locking_mode=EXCLUSIVE; PRAGMA synchronous=FULL; BEGIN EXCLUSIVE",
      );
      const existing = db
        .prepare(
          "SELECT name,sql FROM sqlite_schema WHERE sql IS NOT NULL AND name NOT GLOB 'sqlite_*'",
        )
        .all();
      if (existing.length) {
        if (
          existing.length !== 1 ||
          existing[0]?.name !== "commands" ||
          existing[0]?.sql !== schema
        )
          throw new Error(
            "computer: incompatible receipt schema; preserve the original data directory",
          );
      } else db.exec(schema);
      db.exec("COMMIT");
    } catch (cause) {
      const failures = [cause];
      try {
        if (db.isTransaction) db.exec("ROLLBACK");
      } catch (rollback) {
        failures.push(rollback);
      }
      try {
        db.close();
      } catch (close) {
        failures.push(close);
      }
      if (failures.length === 1) throw cause;
      throw new AggregateError(
        failures,
        "computer: receipt startup cleanup failed",
      );
    }
  }

  #transaction<T>(action: () => T): T {
    if (this.#closed) throw new Error("computer: receipts are closed");
    this.#db.exec("BEGIN IMMEDIATE");
    try {
      const result = action();
      this.#db.exec("COMMIT");
      return result;
    } catch (cause) {
      try {
        this.#db.exec("ROLLBACK");
      } catch (rollback) {
        throw new AggregateError(
          [cause, rollback],
          "computer: receipt transaction rollback failed",
        );
      }
      throw cause;
    }
  }

  #decode(row: Row): ReceiptRecord {
    if (
      typeof row.id !== "string" ||
      (row.kind !== "tool" && row.kind !== "command") ||
      typeof row.method !== "string" ||
      typeof row.input !== "string" ||
      typeof row.digest !== "string" ||
      typeof row.invocation !== "string" ||
      typeof row.admitted_at !== "string" ||
      (row.dispatched !== 0n && row.dispatched !== 1n) ||
      (row.outcome !== null && typeof row.outcome !== "string")
    )
      throw new Error("computer: receipt storage is corrupt");
    const owner = parseJSON(row.invocation);
    invocation(owner);
    if (canonicalJSON(owner) !== row.invocation)
      throw new Error("computer: stored invocation is not canonical");
    const original: OriginalCommand = {
      id: row.id,
      kind: row.kind,
      method: row.method,
      input: row.input,
      inputDigest: row.digest,
      invocation: owner,
      admittedAt: row.admitted_at,
    };
    admitted(original, this.#limits);
    const base = { ...original, dispatched: row.dispatched === 1n };
    if (row.outcome === null) return { ...base, state: "pending" };
    const outcome = parseJSON(row.outcome);
    this.#validateOutcome(original.kind, original.method, outcome);
    if (canonicalJSON(outcome) !== row.outcome)
      throw new Error("computer: stored outcome is not canonical");
    return {
      ...base,
      state: "settled",
      outcome: outcome as CallToolResult | CommandReply,
    };
  }

  read(id: string): ReceiptRecord | undefined {
    if (!validCommand(id)) throw new Error("computer: receipt id is invalid");
    const row = this.#db
      .prepare(`SELECT ${columns} FROM commands WHERE id=?`)
      .get(id);
    return row ? this.#decode(row) : undefined;
  }

  admit(
    original: OriginalCommand,
    admission: "ordinary" | "reserved" = "ordinary",
  ): ReceiptAdmission {
    const { owner, bytes } = admitted(original, this.#limits);
    return this.#transaction(() => {
      const previous = this.read(original.id);
      if (previous) {
        if (
          previous.kind !== original.kind ||
          previous.method !== original.method ||
          previous.input !== original.input ||
          previous.inputDigest !== original.inputDigest ||
          canonicalJSON(previous.invocation) !== owner
        )
          conflict();
        return { fresh: false, record: previous };
      }
      const usage = this.#db
        .prepare(
          "SELECT count(*) AS count, coalesce(sum(length(CAST(id AS BLOB))+length(CAST(kind AS BLOB))+length(CAST(method AS BLOB))+length(CAST(input AS BLOB))+length(CAST(digest AS BLOB))+length(CAST(invocation AS BLOB))+length(CAST(admitted_at AS BLOB))+CASE WHEN outcome IS NULL THEN ? ELSE length(CAST(outcome AS BLOB)) END),0) AS bytes FROM commands",
        )
        .get(this.#limits.maxResultBytes)!;
      const reserve = admission === "ordinary";
      const maxRecords = this.#limits.maxRecords - (reserve ? 1 : 0);
      const maxBytes =
        this.#limits.maxBytes -
        (reserve
          ? this.#limits.maxInputBytes + this.#limits.maxResultBytes
          : 0);
      const added = bytes + this.#limits.maxResultBytes;
      // Every admitted result has space before dispatch. Ordinary work also
      // leaves a complete command slot for an application-authorized revocation.
      if (
        BigInt(usage.count as bigint) >= BigInt(maxRecords) ||
        BigInt(usage.bytes as bigint) + BigInt(added) > BigInt(maxBytes)
      )
        throw new Refusal(
          "receipt_capacity",
          "computer receipt capacity exceeded; retained evidence was not removed",
        );
      this.#db
        .prepare(
          "INSERT INTO commands(id,kind,method,input,digest,invocation,admitted_at) VALUES(?,?,?,?,?,?,?)",
        )
        .run(
          original.id,
          original.kind,
          original.method,
          original.input,
          original.inputDigest,
          owner,
          original.admittedAt,
        );
      return { fresh: true };
    });
  }

  markDispatched(id: string): void {
    this.#transaction(() => {
      const current = this.read(id);
      if (!current || current.state !== "pending" || current.dispatched)
        throw new Error(
          "computer: original command cannot be dispatched again",
        );
      this.#db.prepare("UPDATE commands SET dispatched=1 WHERE id=?").run(id);
    });
  }

  #validateOutcome(
    kind: OriginalCommand["kind"],
    method: string,
    outcome: unknown,
  ): void {
    if (kind === "tool") {
      admitToolResult(outcome);
      projectToolOutput(method, outcome);
    } else commandReply(outcome);
  }

  #settle(id: string, kind: OriginalCommand["kind"], outcome: unknown): void {
    const encoded = canonicalJSON(outcome);
    if (Buffer.byteLength(encoded) > this.#limits.maxResultBytes)
      throw new Error(
        "computer: observed result exceeds retained result capacity",
      );
    this.#transaction(() => {
      const current = this.read(id);
      if (!current || current.kind !== kind)
        throw new Error("computer: settlement requires its original admission");
      this.#validateOutcome(kind, current.method, outcome);
      if (current.state === "settled") {
        if (canonicalJSON(current.outcome) !== encoded) conflict();
        return;
      }
      this.#db
        .prepare("UPDATE commands SET outcome=? WHERE id=?")
        .run(encoded, id);
    });
  }

  settleTool(id: string, result: CallToolResult): void {
    this.#settle(id, "tool", result);
  }

  settleCommand(id: string, reply: CommandReply): void {
    this.#settle(id, "command", reply);
  }

  #original(
    request: ReceiptRequest,
    kind: OriginalCommand["kind"],
  ): ReceiptRecord | undefined {
    if (
      !validCommand(request.method) ||
      !/^[a-f0-9]{64}$/.test(request.inputDigest)
    )
      throw new Error("computer: receipt request is invalid");
    const original = this.read(request.id);
    if (
      original &&
      (original.kind !== kind ||
        original.method !== request.method ||
        original.inputDigest !== request.inputDigest)
    )
      conflict();
    return original;
  }

  receipt(request: ReceiptRequest): { found: boolean; reply?: CommandReply } {
    const original = this.#original(request, "command");
    return original?.state === "settled"
      ? { found: true, reply: original.outcome as CommandReply }
      : { found: false };
  }

  toolReceipt(request: ReceiptRequest): ToolReceipt {
    const original = this.#original(request, "tool");
    if (original?.state !== "settled") return { found: false };
    const result = original.outcome as CallToolResult;
    return {
      found: true,
      output: canonicalJSON(projectToolOutput(original.method, result)),
      ...(result.isError ? { isError: true } : {}),
    };
  }

  list(input: { before?: string; limit: number }): ReceiptPage {
    if (
      !Number.isInteger(input.limit) ||
      input.limit < 1 ||
      input.limit > this.#limits.maxRecords ||
      (input.before !== undefined && !validCommand(input.before))
    )
      throw new Refusal(
        "invalid_receipt_page",
        "receipt page bounds are invalid",
      );
    const cursor = input.before
      ? this.#db
          .prepare("SELECT sequence FROM commands WHERE id=?")
          .get(input.before)
      : undefined;
    if (input.before && !cursor)
      throw new Refusal(
        "receipt_not_found",
        "receipt page cursor does not exist",
      );
    const headers =
      "id,kind,method,admitted_at,dispatched,outcome IS NOT NULL AS settled";
    const rows = cursor
      ? this.#db
          .prepare(
            `SELECT ${headers} FROM commands WHERE sequence < ? ORDER BY sequence DESC LIMIT ?`,
          )
          .all(cursor.sequence!, input.limit + 1)
      : this.#db
          .prepare(
            `SELECT ${headers} FROM commands ORDER BY sequence DESC LIMIT ?`,
          )
          .all(input.limit + 1);
    const records = rows.slice(0, input.limit).map(summary);
    return {
      records,
      ...(rows.length > input.limit ? { nextCursor: records.at(-1)!.id } : {}),
    };
  }

  close(): void {
    if (this.#closed) return;
    this.#db.close();
    this.#closed = true;
  }
}
