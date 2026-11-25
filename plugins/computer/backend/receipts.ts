import type { SQLOutputValue } from "node:sqlite";
import { openPrivateDatabase, type Database } from "@flame/backend-sdk/sqlite";
import { isAbsolute } from "node:path";
import {
  Refusal,
  admittedInvocation,
  invocationConflictCode,
  InvocationScope,
  canonicalJSON,
  digestJSON,
  parseJSON,
  type Invocation,
  type ReceiptRequest,
} from "@flame/backend-sdk/mcp";
import { validCommand } from "@flame/backend-sdk/mcp/contract";
import type { ToolReceipt } from "@flame/backend-sdk";
import type { CallToolResult } from "@modelcontextprotocol/sdk/types.js";
import { admitToolResult, projectToolOutput } from "./tool-result";
import { evidenceBytes, receiptUsageSQL } from "./receiptEvidence";

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
    invocationConflictCode,
    "original computer command identity binds different evidence",
  );
}

function object(value: unknown): asserts value is Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("computer: receipt evidence must be an object");
}

// Callers pass originals built from SDK-admitted call context; only the
// receipt's own capacity remains to be enforced.
function evidence(
  original: OriginalCommand,
  limits: ReceiptLimits,
): { owner: string; bytes: number } {
  const owner = canonicalJSON(original.invocation);
  const bytes = evidenceBytes({ ...original, invocation: owner });
  if (bytes > limits.maxInputBytes)
    throw new Refusal(
      "input_capacity",
      "computer input capacity exceeded by original command evidence",
    );
  return { owner, bytes };
}

function storedOriginal(row: ReceiptRow): OriginalCommand {
  const invocation = admittedInvocation(
    parseJSON(row.invocation),
    InvocationScope.Installation,
  );
  const input = parseJSON(row.input);
  if (
    !invocation ||
    canonicalJSON(invocation) !== row.invocation ||
    !validCommand(row.id) ||
    !validCommand(row.method) ||
    !/^[a-f0-9]{64}$/.test(row.digest) ||
    !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$/.test(row.admitted_at) ||
    !Number.isFinite(Date.parse(row.admitted_at)) ||
    new Date(row.admitted_at).toISOString() !== row.admitted_at ||
    canonicalJSON(input) !== row.input ||
    digestJSON(input) !== row.digest
  )
    throw new Error("computer: stored receipt evidence is invalid");
  return {
    id: row.id,
    kind: row.kind,
    method: row.method,
    input: row.input,
    inputDigest: row.digest,
    invocation,
    admittedAt: row.admitted_at,
  };
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

interface ReceiptRow {
  id: string;
  kind: "tool" | "command";
  method: string;
  input: string;
  digest: string;
  invocation: string;
  admitted_at: string;
  dispatched: 0n | 1n;
  outcome: string | null;
}

function assertReceiptRow(row: Row): asserts row is Row & ReceiptRow {
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
  readonly #db: Database;
  readonly #limits: ReceiptLimits;

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
    this.#db = openPrivateDatabase(path, "computer", {
      objects: { commands: schema },
    });
  }

  #decode(row: Row): ReceiptRecord {
    assertReceiptRow(row);
    const original = storedOriginal(row);
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

  #usage() {
    // SQLite reads a column's octet length from its record metadata, including
    // large media results, without materializing another payload in the UI path.
    return this.#db
      .prepare(`SELECT ${receiptUsageSQL} FROM commands`)
      .get(this.#limits.maxResultBytes)!;
  }

  capacity() {
    const usage = this.#usage();
    return {
      records: Number(usage.count),
      bytes: Number(usage.bytes),
      pending: Number(usage.pending),
      maxRecords: this.#limits.maxRecords,
      maxBytes: this.#limits.maxBytes,
    };
  }

  admit(
    original: OriginalCommand,
    admission: "ordinary" | "reserved" = "ordinary",
  ): ReceiptAdmission {
    const { owner, bytes } = evidence(original, this.#limits);
    return this.#db.transaction(() => {
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
      const usage = this.#usage();
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
    this.#db.transaction(() => {
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
    this.#db.transaction(() => {
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
    if (original?.state !== "settled") return {};
    const result = original.outcome as CallToolResult;
    return {
      result: {
        output: projectToolOutput(original.method, result),
        ...(result.isError ? { isError: true } : {}),
      },
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
    this.#db.close();
  }
}
