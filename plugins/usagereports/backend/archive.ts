import { DatabaseSync, type SQLOutputValue } from "node:sqlite";
import { Temporal } from "@js-temporal/polyfill";
import {
  Capability,
  Refusal,
  canonicalJSON,
  decodeReply,
  digestJSON,
  parseJSON,
  validCommand,
  type CallContext,
  type Method,
} from "@flame/backend-sdk";
import { validateWire } from "@flame/runtime-client/validation";

export const MAX_REPORTS = 64;
export const MAX_REPORT_BYTES = 1 << 20;
type Period = "daily" | "weekly";
type RecordValue = Record<string, unknown>;
type Command = Pick<
  CallContext,
  "commandId" | "requestInput" | "inputDigest" | "signal"
>;
type Header = { id: string; period: Period; observedAt: string; since: string };
type Snapshot = {
  id: string;
  command: string;
  period: Period;
  value: RecordValue;
};
type Row = Record<string, SQLOutputValue>;

const schemas = {
  commands:
    "CREATE TABLE commands (id TEXT PRIMARY KEY NOT NULL, method TEXT NOT NULL, input TEXT NOT NULL CHECK(json_valid(input)), digest TEXT NOT NULL, reply TEXT CHECK(reply IS NULL OR json_valid(reply)))",
  reports:
    "CREATE TABLE reports (id TEXT PRIMARY KEY NOT NULL, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))",
};
function object(
  value: unknown,
  fields?: string[],
): asserts value is RecordValue {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    (fields &&
      (Object.keys(value).length !== fields.length ||
        fields.some((field) => !Object.hasOwn(value, field))))
  )
    throw new Error("usage reports: invalid stored evidence");
}
function timestamp(value: unknown): Temporal.Instant {
  if (
    typeof value !== "string" ||
    !/^\d{4}-\d\d-\d\dT\d\d:\d\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(
      value,
    )
  )
    throw new Error("usage reports: accounting timestamp is invalid");
  const instant = Temporal.Instant.from(value);
  if (instant.equals(Temporal.Instant.from("0001-01-01T00:00:00Z")))
    throw new Error("usage reports: accounting timestamp is zero");
  return instant;
}
function summary(value: unknown): asserts value is RecordValue {
  object(value);
  if (validateWire("UsageSummary", value).length)
    throw new Error("usage reports: invalid accounting wire evidence");
  if (
    Temporal.Instant.compare(
      timestamp(value.since),
      timestamp(value.observedAt),
    ) > 0
  )
    throw new Error(
      "usage reports: accounting range ends before its lower bound",
    );
}
function header(snapshot: Snapshot): Header {
  return {
    id: snapshot.id,
    period: snapshot.period,
    observedAt: snapshot.value.observedAt as string,
    since: snapshot.value.since as string,
  };
}
function reportId(command: string, period: Period, value: unknown): string {
  return digestJSON({ Command: command, Period: period, Value: value });
}
function admitHeader(value: unknown): void {
  object(value);
  if (
    typeof value.id !== "string" ||
    !/^[a-f0-9]{64}$/.test(value.id) ||
    Temporal.Instant.compare(
      timestamp(value.since),
      timestamp(value.observedAt),
    ) > 0
  )
    throw new Error("usage reports: invalid saved report header");
}
function conflict(): never {
  throw new Refusal(
    "command_conflict",
    "original command identity binds different input",
  );
}
function unconfirmed(): never {
  throw new Error("usage reports: original command outcome is unconfirmed");
}

export class Archive {
  readonly #db: DatabaseSync;
  readonly #capture: Method;
  readonly #remove: Method;
  readonly #get: Method;
  readonly #commands: ReadonlyMap<string, Method>;
  #closed = false;

  constructor(path: string, contract: Capability) {
    this.#capture = contract.method("flame.usage-reports/capture.v1");
    this.#remove = contract.method("flame.usage-reports/delete.v1");
    this.#get = contract.method("flame.usage-reports/get.v1");
    this.#commands = new Map(
      [this.#capture, this.#remove].map((method) => [
        method.descriptor.name,
        method,
      ]),
    );
    const db = new DatabaseSync(path, {
      allowExtension: false,
      enableDoubleQuotedStringLiterals: false,
      readBigInts: true,
      timeout: 0,
    });
    this.#db = db;
    try {
      // EXCLUSIVE retains the SQLite file lock between transactions. Admission
      // survives a lost callback without holding a SQL transaction over await.
      db.exec(
        "PRAGMA locking_mode=EXCLUSIVE; PRAGMA synchronous=FULL; BEGIN EXCLUSIVE",
      );
      try {
        const stored = db
          .prepare(
            "SELECT name, sql FROM sqlite_schema WHERE sql IS NOT NULL AND name NOT GLOB 'sqlite_*'",
          )
          .all();
        if (stored.length) {
          if (
            stored.length !== Object.keys(schemas).length ||
            stored.some(
              (row) =>
                typeof row.name !== "string" ||
                !Object.hasOwn(schemas, row.name) ||
                row.sql !== schemas[row.name as keyof typeof schemas],
            )
          )
            throw new Error(
              "usage reports: incompatible archive schema; use a fresh private data directory",
            );
        } else for (const sql of Object.values(schemas)) db.exec(sql);
        db.exec("COMMIT");
      } catch (error) {
        this.#rollback(error);
      }
    } catch (error) {
      db.close();
      throw error;
    }
  }

  #rollback(error: unknown): never {
    try {
      this.#db.exec("ROLLBACK");
    } catch (rollback) {
      throw new AggregateError(
        [error, rollback],
        "usage reports: transaction outcome is unconfirmed",
      );
    }
    throw error;
  }
  #transaction<T>(action: () => T): T {
    if (this.#closed) throw new Error("usage reports: archive is closed");
    this.#db.exec("BEGIN IMMEDIATE");
    try {
      const result = action();
      this.#db.exec("COMMIT");
      return result;
    } catch (error) {
      return this.#rollback(error);
    }
  }
  #command(id: string): Row | undefined {
    if (!validCommand(id))
      throw new Error("usage reports: invalid stored command identity");
    const row = this.#db
      .prepare("SELECT id,method,input,digest,reply FROM commands WHERE id=?")
      .get(id);
    if (!row) return undefined;
    if (
      row.id !== id ||
      typeof row.method !== "string" ||
      typeof row.input !== "string" ||
      typeof row.digest !== "string" ||
      (row.reply !== null && typeof row.reply !== "string")
    )
      throw new Error("usage reports: invalid stored command");
    const method = this.#commands.get(row.method);
    if (!method)
      throw new Error("usage reports: stored command method is not published");
    const input = parseJSON(row.input);
    method.validateInput(input);
    if (canonicalJSON(input) !== row.input || digestJSON(input) !== row.digest)
      throw new Error("usage reports: corrupt original command evidence");
    if (row.reply !== null) {
      const reply = this.#reply(method, row.reply as string) as RecordValue;
      if (
        method === this.#capture &&
        Object.hasOwn(reply, "value") &&
        (reply.value as Header).period !== (input as { period: Period }).period
      )
        throw new Error(
          "usage reports: saved outcome differs from its original period",
        );
    }
    return row;
  }
  #reply(method: Method, raw: string): unknown {
    const reply = parseJSON(raw);
    if (canonicalJSON(reply) !== raw)
      throw new Error("usage reports: corrupt original outcome");
    try {
      const output = decodeReply(reply);
      method.validateOutput(output);
      if (method === this.#capture) admitHeader(output);
      else if (output !== true)
        throw new Error("usage reports: invalid saved removal outcome");
    } catch (error) {
      if (
        !(error instanceof Refusal) ||
        !["invalid_report", "archive_capacity", "report_not_found"].includes(
          error.code,
        )
      )
        throw error;
    }
    return reply;
  }
  #admit(
    method: Method,
    context: Command,
  ): { fresh: true } | { fresh: false; reply: unknown } {
    const id = context.commandId;
    if (!validCommand(id))
      throw new Error("usage reports: original command is required");
    const input = parseJSON(context.requestInput);
    method.validateInput(input);
    if (
      canonicalJSON(input) !== context.requestInput ||
      digestJSON(input) !== context.inputDigest
    )
      throw new Error("usage reports: original input evidence differs");
    const previous = this.#command(id);
    if (previous) {
      if (
        previous.method !== method.descriptor.name ||
        previous.input !== context.requestInput ||
        previous.digest !== context.inputDigest
      )
        conflict();
      if (previous.reply === null) unconfirmed();
      return {
        fresh: false,
        reply: this.#reply(method, previous.reply as string),
      };
    }
    this.#db
      .prepare("INSERT INTO commands(id,method,input,digest) VALUES(?,?,?,?)")
      .run(
        id,
        method.descriptor.name,
        context.requestInput,
        context.inputDigest,
      );
    return { fresh: true };
  }
  #settle(method: Method, context: Command, reply: unknown): unknown {
    const raw = canonicalJSON(reply);
    this.#reply(method, raw);
    const result = this.#db
      .prepare(
        "UPDATE commands SET reply=? WHERE id=? AND method=? AND input=? AND digest=? AND reply IS NULL",
      )
      .run(
        raw,
        context.commandId!,
        method.descriptor.name,
        context.requestInput,
        context.inputDigest,
      );
    if (result.changes !== 1n)
      throw new Error("usage reports: original admission is not pending");
    return reply;
  }

  async capture(
    context: Command,
    read: (days: number) => Promise<string>,
  ): Promise<Header> {
    context.signal.throwIfAborted();
    const admission = this.#transaction(() =>
      this.#admit(this.#capture, context),
    );
    if (!admission.fresh) return decodeReply(admission.reply) as Header;
    const input = parseJSON(context.requestInput) as { period: Period };
    const value = parseJSON(await read(input.period === "daily" ? 1 : 7));
    context.signal.throwIfAborted();
    summary(value);
    const snapshot: Snapshot = {
      id: reportId(context.commandId!, input.period, value),
      command: context.commandId!,
      period: input.period,
      value,
    };
    const reply = this.#transaction(() => {
      context.signal.throwIfAborted();
      const count = this.#db
        .prepare("SELECT COUNT(*) AS count FROM reports")
        .get()!.count as bigint;
      if (count > BigInt(MAX_REPORTS))
        throw new Error("usage reports: invalid stored archive capacity");
      let refusal: Refusal | undefined;
      if (Buffer.byteLength(canonicalJSON(value)) > MAX_REPORT_BYTES)
        refusal = new Refusal(
          "invalid_report",
          "captured accounting exceeds the report capacity",
        );
      else if (count === BigInt(MAX_REPORTS))
        refusal = new Refusal(
          "archive_capacity",
          "remove a saved report before capturing another",
        );
      if (refusal)
        return this.#settle(this.#capture, context, {
          failure: { code: refusal.code, message: refusal.message },
        });
      this.#get.validateOutput({ ...header(snapshot), summary: value });
      this.#db
        .prepare("INSERT INTO reports(id,snapshot) VALUES(?,?)")
        .run(snapshot.id, canonicalJSON(snapshot));
      return this.#settle(this.#capture, context, { value: header(snapshot) });
    });
    return decodeReply(reply) as Header;
  }

  #report(id: string, raw: string): { snapshot: Snapshot; header: Header } {
    const snapshot = parseJSON(raw);
    object(snapshot, ["id", "command", "period", "value"]);
    if (
      snapshot.id !== id ||
      typeof snapshot.command !== "string" ||
      (snapshot.period !== "daily" && snapshot.period !== "weekly")
    )
      throw new Error("usage reports: corrupt saved report identity");
    summary(snapshot.value);
    if (
      Buffer.byteLength(canonicalJSON(snapshot.value)) > MAX_REPORT_BYTES ||
      reportId(snapshot.command, snapshot.period, snapshot.value) !== id
    )
      throw new Error("usage reports: corrupt saved report content");
    const original = this.#command(snapshot.command);
    if (
      !original ||
      original.method !== this.#capture.descriptor.name ||
      original.reply === null ||
      original.input !== canonicalJSON({ period: snapshot.period })
    )
      throw new Error("usage reports: saved report lacks original settlement");
    const projected = header(snapshot as Snapshot);
    if (
      canonicalJSON(
        decodeReply(this.#reply(this.#capture, original.reply as string)),
      ) !== canonicalJSON(projected)
    )
      throw new Error(
        "usage reports: saved report differs from its settlement",
      );
    this.#get.validateOutput({ ...projected, summary: snapshot.value });
    return { snapshot: snapshot as Snapshot, header: projected };
  }
  read(id: string): RecordValue {
    const row = this.#db
      .prepare("SELECT snapshot FROM reports WHERE id=?")
      .get(id);
    if (!row)
      throw new Refusal("report_not_found", "saved report was not found");
    if (typeof row.snapshot !== "string")
      throw new Error("usage reports: invalid stored report");
    const report = this.#report(id, row.snapshot);
    return { ...report.header, summary: report.snapshot.value };
  }
  list(): Header[] {
    const rows = this.#db
      .prepare("SELECT id,snapshot FROM reports ORDER BY rowid DESC LIMIT ?")
      .all(MAX_REPORTS + 1);
    if (rows.length > MAX_REPORTS)
      throw new Error("usage reports: invalid stored archive capacity");
    return rows.map((row) => {
      if (typeof row.id !== "string" || typeof row.snapshot !== "string")
        throw new Error("usage reports: invalid stored report");
      return this.#report(row.id, row.snapshot).header;
    });
  }
  remove(context: Command): boolean {
    context.signal.throwIfAborted();
    return decodeReply(
      this.#transaction(() => {
        const admission = this.#admit(this.#remove, context);
        if (!admission.fresh) return admission.reply;
        const { id } = parseJSON(context.requestInput) as { id: string };
        try {
          this.read(id);
          const removed = this.#db
            .prepare("DELETE FROM reports WHERE id=?")
            .run(id);
          if (removed.changes !== 1n)
            throw new Error("usage reports: report removal is unconfirmed");
          return this.#settle(this.#remove, context, { value: true });
        } catch (error) {
          if (!(error instanceof Refusal) || error.code !== "report_not_found")
            throw error;
          return this.#settle(this.#remove, context, {
            failure: { code: error.code, message: error.message },
          });
        }
      }),
    ) as boolean;
  }
  receipt(request: {
    id: string;
    method: string;
    inputDigest: string;
  }): unknown {
    const original = this.#command(request.id);
    if (!original) return { found: false };
    if (
      original.method !== request.method ||
      original.digest !== request.inputDigest
    )
      conflict();
    if (original.reply === null) return { found: false };
    return {
      found: true,
      reply: this.#reply(
        this.#commands.get(request.method)!,
        original.reply as string,
      ),
    };
  }
  close(): void {
    if (this.#closed) return;
    this.#db.close();
    this.#closed = true;
  }
}
