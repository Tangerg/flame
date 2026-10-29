import { DatabaseSync, type SQLOutputValue } from "node:sqlite";
import { isAbsolute } from "node:path";
import {
  Refusal,
  canonicalJSON,
  decodeReply,
  digestJSON,
  parseJSON,
  type ReceiptRequest,
  type Invocation,
} from "@qimu/node";
import { validCommand } from "@qimu/node/contract";
import { LIMITS } from "../contracts/limits";
import {
  schemas,
  type Asset,
  type Assessment,
  type Header,
  type Revision,
} from "../contracts/capability";
import { Document, digest } from "./document";

export interface Original {
  id: string;
  kind: "command" | "tool";
  method: string;
  input: string;
  inputDigest: string;
  invocation: Invocation;
}
export type Reply =
  { value: unknown } | { failure: { code: string; message: string } };
type Row = Record<string, SQLOutputValue>;

const tables = Object.freeze({
  commands:
    "CREATE TABLE commands (sequence INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('command','tool')), method TEXT NOT NULL, input TEXT NOT NULL CHECK(json_valid(input)), digest TEXT NOT NULL, invocation TEXT NOT NULL CHECK(json_valid(invocation)), reply TEXT CHECK(reply IS NULL OR json_valid(reply)))",
  documents:
    "CREATE TABLE documents (sequence INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, header TEXT NOT NULL CHECK(json_valid(header)))",
  revisions:
    "CREATE TABLE revisions (sequence INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE, metadata TEXT NOT NULL CHECK(json_valid(metadata)), bytes BLOB NOT NULL, inspection TEXT NOT NULL CHECK(json_valid(inspection)), processing TEXT CHECK(processing IS NULL OR json_valid(processing)))",
  assets:
    "CREATE TABLE assets (id TEXT PRIMARY KEY, document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE, revision_id TEXT NOT NULL REFERENCES revisions(id) ON DELETE CASCADE, metadata TEXT NOT NULL CHECK(json_valid(metadata)), bytes BLOB NOT NULL)",
  assessments:
    "CREATE TABLE assessments (sequence INTEGER PRIMARY KEY, id TEXT UNIQUE NOT NULL, document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE, revision_id TEXT NOT NULL REFERENCES revisions(id) ON DELETE CASCADE, payload TEXT NOT NULL CHECK(json_valid(payload)))",
  uploads:
    "CREATE TABLE uploads (id TEXT PRIMARY KEY, size INTEGER NOT NULL, digest TEXT NOT NULL, bytes BLOB NOT NULL)",
});

function text(value: SQLOutputValue | undefined): string {
  if (typeof value !== "string")
    throw new Error("deliverables: invalid archive text");
  return value;
}
function bytes(value: SQLOutputValue | undefined): Uint8Array {
  if (!(value instanceof Uint8Array))
    throw new Error("deliverables: invalid archive bytes");
  return value;
}
function json(value: SQLOutputValue | undefined): unknown {
  const raw = text(value);
  const decoded: unknown = JSON.parse(raw);
  if (canonicalJSON(decoded) !== raw)
    throw new Error("deliverables: archive JSON is not canonical");
  return decoded;
}
function conflict(): never {
  throw new Refusal(
    "command_conflict",
    "original command identity binds different evidence",
  );
}

export function toolResult(reply: Reply) {
  const value = "value" in reply ? { value: reply.value } : reply;
  return {
    content: [{ type: "text" as const, text: canonicalJSON(value) }],
    structuredContent: value,
    ...("failure" in reply ? { isError: true } : {}),
  };
}

export class Archive {
  readonly #db: DatabaseSync;
  #closed = false;

  constructor(path: string) {
    if (!isAbsolute(path))
      throw new Error("deliverables: absolute archive path is required");
    const db = new DatabaseSync(path, {
      allowExtension: false,
      enableDoubleQuotedStringLiterals: false,
      readBigInts: true,
      timeout: 0,
    });
    this.#db = db;
    try {
      db.exec(
        "PRAGMA foreign_keys=ON; PRAGMA locking_mode=EXCLUSIVE; PRAGMA synchronous=FULL; PRAGMA auto_vacuum=FULL; PRAGMA max_page_count=262144; BEGIN EXCLUSIVE",
      );
      const existing = db
        .prepare(
          "SELECT name,sql FROM sqlite_schema WHERE sql IS NOT NULL AND name NOT GLOB 'sqlite_*'",
        )
        .all();
      if (existing.length) {
        if (
          existing.length !== Object.keys(tables).length ||
          existing.some(
            (row) =>
              typeof row.name !== "string" ||
              !Object.hasOwn(tables, row.name) ||
              row.sql !== tables[row.name as keyof typeof tables],
          )
        )
          throw new Error(
            "deliverables: incompatible archive schema; preserve the original data directory",
          );
      } else for (const sql of Object.values(tables)) db.exec(sql);
      db.exec("COMMIT");
    } catch (cause) {
      try {
        if (db.isTransaction) db.exec("ROLLBACK");
      } finally {
        db.close();
      }
      throw cause;
    }
  }

  #transaction<T>(work: () => T): T {
    if (this.#closed) throw new Error("deliverables: archive is closed");
    this.#db.exec("BEGIN IMMEDIATE");
    try {
      const result = work();
      this.#db.exec("COMMIT");
      return result;
    } catch (cause) {
      try {
        this.#db.exec("ROLLBACK");
      } catch (rollback) {
        throw new AggregateError(
          [cause, rollback],
          "deliverables: archive rollback is unconfirmed",
        );
      }
      throw cause;
    }
  }

  #writing(): void {
    if (!this.#db.isTransaction)
      throw new Error(
        "deliverables: publication requires its original settlement transaction",
      );
  }

  #capacity(extra = 0): void {
    const usage = this.#db
      .prepare(
        `SELECT
      (SELECT coalesce(sum(length(CAST(input AS BLOB))+length(CAST(invocation AS BLOB))+coalesce(length(CAST(reply AS BLOB)),?)),0) FROM commands) +
      (SELECT coalesce(sum(length(bytes)+length(CAST(inspection AS BLOB))+length(CAST(metadata AS BLOB))+coalesce(length(CAST(processing AS BLOB)),0)),0) FROM revisions) +
      (SELECT coalesce(sum(length(bytes)+length(CAST(metadata AS BLOB))),0) FROM assets) +
      (SELECT coalesce(sum(length(CAST(payload AS BLOB))),0) FROM assessments) +
      (SELECT coalesce(sum(size),0) FROM uploads) AS used`,
      )
      .get(BigInt(LIMITS.reportBytes * 2))!;
    if ((usage.used as bigint) + BigInt(extra) > BigInt(LIMITS.archiveBytes))
      throw new Refusal(
        "archive_capacity",
        "deliverable archive capacity is exhausted; remove retained files or discard uploads",
      );
  }

  #original(original: Original): void {
    if (
      !validCommand(original.id) ||
      !validCommand(original.method) ||
      !["tool", "command"].includes(original.kind) ||
      !validCommand(original.invocation.id) ||
      !["human", "model", "automatic"].includes(original.invocation.origin)
    )
      throw new Error(
        "deliverables: trusted original command identity is required",
      );
    const input = parseJSON(original.input);
    if (
      canonicalJSON(input) !== original.input ||
      digestJSON(input) !== original.inputDigest
    )
      throw new Error(
        "deliverables: original command evidence differs from its digest",
      );
    if (
      Buffer.byteLength(original.input) >
      LIMITS.inputBytes + LIMITS.chunkBytes
    )
      throw new Refusal(
        "input_capacity",
        "original input exceeds the deliverable command capacity",
      );
  }

  #reply(row: Row): Reply | undefined {
    const original: Original = {
      id: text(row.id),
      kind: text(row.kind) as Original["kind"],
      method: text(row.method),
      input: text(row.input),
      inputDigest: text(row.digest),
      invocation: json(row.invocation) as Invocation,
    };
    this.#original(original);
    if (row.reply === null) return undefined;
    const reply = json(row.reply) as Reply;
    if (
      !reply ||
      typeof reply !== "object" ||
      Array.isArray(reply) ||
      Object.keys(reply).length !== 1 ||
      (!("value" in reply) &&
        (!("failure" in reply) ||
          !reply.failure ||
          Object.keys(reply.failure).length !== 2 ||
          typeof reply.failure.code !== "string" ||
          !reply.failure.code ||
          typeof reply.failure.message !== "string"))
    )
      throw new Error("deliverables: original outcome is corrupt");
    return reply;
  }

  async command(
    original: Original,
    signal: AbortSignal,
    prepare: (id: string) => Promise<() => unknown> | (() => unknown),
    capacity: "ordinary" | "reclaim" = "ordinary",
  ): Promise<unknown> {
    signal.throwIfAborted();
    this.#original(original);
    const admission = this.#transaction(() => {
      const previous = this.#db
        .prepare("SELECT * FROM commands WHERE id=?")
        .get(original.id);
      if (previous) {
        if (
          previous.kind !== original.kind ||
          previous.method !== original.method ||
          previous.input !== original.input ||
          previous.digest !== original.inputDigest ||
          previous.invocation !== canonicalJSON(original.invocation)
        )
          conflict();
        const reply = this.#reply(previous);
        if (!reply)
          throw new Error(
            "deliverables: original command outcome is unconfirmed; never replay it",
          );
        return { fresh: false as const, reply };
      }
      const reserve =
        capacity === "ordinary"
          ? LIMITS.inputBytes + LIMITS.reportBytes * 2 + 4096
          : 0;
      if (
        (this.#db.prepare("SELECT count(*) AS n FROM commands").get()!
          .n as bigint) >=
        BigInt(LIMITS.records - (capacity === "ordinary" ? 1 : 0))
      )
        throw new Refusal(
          "receipt_capacity",
          "retained original command capacity is exhausted",
        );
      this.#capacity(
        Buffer.byteLength(original.input) +
          Buffer.byteLength(canonicalJSON(original.invocation)) +
          LIMITS.reportBytes * 2 +
          reserve,
      );
      this.#db
        .prepare(
          "INSERT INTO commands(id,kind,method,input,digest,invocation) VALUES(?,?,?,?,?,?)",
        )
        .run(
          original.id,
          original.kind,
          original.method,
          original.input,
          original.inputDigest,
          canonicalJSON(original.invocation),
        );
      return { fresh: true as const };
    });
    if (!admission.fresh) return decodeReply(admission.reply);
    try {
      const publish = await prepare(original.id);
      signal.throwIfAborted();
      return decodeReply(
        this.#transaction(() => {
          signal.throwIfAborted();
          const value = publish();
          const reply: Reply = { value };
          this.#settle(original.id, reply);
          this.#capacity(
            capacity === "ordinary"
              ? LIMITS.inputBytes + LIMITS.reportBytes * 2 + 4096
              : 0,
          );
          return reply;
        }),
      );
    } catch (cause) {
      if (cause instanceof Refusal)
        this.#transaction(() =>
          this.#settle(original.id, {
            failure: { code: cause.code, message: cause.message },
          }),
        );
      throw cause;
    }
  }

  #settle(id: string, reply: Reply): void {
    const encoded = canonicalJSON(reply);
    if (Buffer.byteLength(encoded) > LIMITS.reportBytes * 2)
      throw new Error(
        "deliverables: original result exceeds the reserved capacity",
      );
    const result = this.#db
      .prepare("UPDATE commands SET reply=? WHERE id=? AND reply IS NULL")
      .run(encoded, id);
    if (result.changes !== 1n)
      throw new Error(
        "deliverables: command settlement requires its pending admission",
      );
  }

  receipt(
    request: ReceiptRequest,
    kind: Original["kind"],
  ): { found: boolean; reply?: Reply } {
    const row = this.#db
      .prepare("SELECT * FROM commands WHERE id=?")
      .get(request.id);
    if (!row) return { found: false };
    if (
      row.method !== request.method ||
      row.kind !== kind ||
      row.digest !== request.inputDigest
    )
      conflict();
    const reply = this.#reply(row);
    return reply ? { found: true, reply } : { found: false };
  }

  document(id: string): Document {
    const row = this.#db
      .prepare("SELECT header FROM documents WHERE id=?")
      .get(id);
    if (!row)
      throw new Refusal("document_not_found", "deliverable was not found");
    const owner = new Document(json(row.header) as Header);
    if (owner.snapshot().id !== id)
      throw new Error("deliverables: document identity is corrupt");
    return owner;
  }

  revision(
    documentId: string,
    id: string,
  ): {
    metadata: Revision;
    bytes: Uint8Array;
    inspection: unknown;
    processing?: unknown;
  } {
    const row = this.#db
      .prepare("SELECT * FROM revisions WHERE document_id=? AND id=?")
      .get(documentId, id);
    if (!row)
      throw new Refusal(
        "revision_not_found",
        "native revision was not found in this deliverable",
      );
    const metadata = schemas.revision.parse(json(row.metadata));
    const content = bytes(row.bytes);
    if (
      metadata.id !== id ||
      metadata.documentId !== documentId ||
      metadata.size !== content.byteLength ||
      metadata.sha256 !== digest(content) ||
      metadata.native.id !== id ||
      metadata.native.sha256 !== metadata.sha256 ||
      metadata.native.size !== metadata.size
    )
      throw new Error(
        "deliverables: native revision identity or bytes are corrupt",
      );
    return {
      metadata,
      bytes: content,
      inspection: json(row.inspection),
      ...(row.processing === null ? {} : { processing: json(row.processing) }),
    };
  }

  publish(
    owner: Document,
    revision: Revision,
    content: Uint8Array,
    inspection: unknown,
    expected?: string,
    processing?: unknown,
  ): Revision {
    this.#writing();
    schemas.revision.parse(revision);
    if (
      revision.size !== content.byteLength ||
      revision.sha256 !== digest(content) ||
      Buffer.byteLength(canonicalJSON(inspection)) > LIMITS.reportBytes
    )
      throw new Error(
        "deliverables: publication differs from admitted native bytes",
      );
    const count = this.#db.prepare("SELECT count(*) AS n FROM revisions").get()!
      .n as bigint;
    if (count >= BigInt(LIMITS.revisions))
      throw new Refusal(
        "revision_capacity",
        "retained revision capacity is exhausted",
      );
    const header = owner.snapshot();
    if (expected) {
      const next = this.document(header.id)
        .revise(expected, revision)
        .snapshot();
      if (canonicalJSON(next) !== canonicalJSON(header))
        throw new Error(
          "deliverables: document transition differs from publication",
        );
      this.#db
        .prepare("UPDATE documents SET header=? WHERE id=?")
        .run(canonicalJSON(header), header.id);
    } else {
      if (
        (this.#db.prepare("SELECT count(*) AS n FROM documents").get()!
          .n as bigint) >= BigInt(LIMITS.documents)
      )
        throw new Refusal(
          "document_capacity",
          "retained deliverable capacity is exhausted",
        );
      this.#db
        .prepare("INSERT INTO documents(id,header) VALUES(?,?)")
        .run(header.id, canonicalJSON(header));
    }
    this.#db
      .prepare(
        "INSERT INTO revisions(id,document_id,metadata,bytes,inspection,processing) VALUES(?,?,?,?,?,?)",
      )
      .run(
        revision.id,
        header.id,
        canonicalJSON(revision),
        content,
        canonicalJSON(inspection),
        processing === undefined ? null : canonicalJSON(processing),
      );
    return revision;
  }

  list(
    before: string | undefined,
    limit: number,
  ): { entries: Header[]; next?: string } {
    const cursor = before
      ? this.#db
          .prepare("SELECT sequence FROM documents WHERE id=?")
          .get(before)
      : undefined;
    if (before && !cursor)
      throw new Refusal(
        "cursor_not_found",
        "deliverable cursor is no longer retained",
      );
    const rows = cursor
      ? this.#db
          .prepare(
            "SELECT id,header FROM documents WHERE sequence < ? ORDER BY sequence DESC LIMIT ?",
          )
          .all(cursor.sequence!, limit + 1)
      : this.#db
          .prepare(
            "SELECT id,header FROM documents ORDER BY sequence DESC LIMIT ?",
          )
          .all(limit + 1);
    const entries = rows
      .slice(0, limit)
      .map((row) => this.document(text(row.id)).snapshot());
    return {
      entries,
      ...(rows.length > limit ? { next: entries.at(-1)!.id } : {}),
    };
  }

  history(
    documentId: string,
    before: string | undefined,
    limit: number,
  ): { entries: Revision[]; next?: string } {
    this.document(documentId);
    const cursor = before
      ? this.#db
          .prepare(
            "SELECT sequence FROM revisions WHERE id=? AND document_id=?",
          )
          .get(before, documentId)
      : undefined;
    if (before && !cursor)
      throw new Refusal(
        "cursor_not_found",
        "revision cursor was not found in this deliverable",
      );
    const rows = cursor
      ? this.#db
          .prepare(
            "SELECT metadata FROM revisions WHERE document_id=? AND sequence < ? ORDER BY sequence DESC LIMIT ?",
          )
          .all(documentId, cursor.sequence!, limit + 1)
      : this.#db
          .prepare(
            "SELECT metadata FROM revisions WHERE document_id=? ORDER BY sequence DESC LIMIT ?",
          )
          .all(documentId, limit + 1);
    const entries = rows
      .slice(0, limit)
      .map((row) => schemas.revision.parse(json(row.metadata)));
    return {
      entries,
      ...(rows.length > limit ? { next: entries.at(-1)!.id } : {}),
    };
  }

  assessments(documentId: string, revisionId: string): Assessment[] {
    return this.#db
      .prepare(
        "SELECT payload FROM assessments WHERE document_id=? AND revision_id=? ORDER BY sequence",
      )
      .all(documentId, revisionId)
      .map((row) => schemas.assessment.parse(json(row.payload)));
  }

  assess(
    documentId: string,
    value: Assessment,
    assets: Array<{ metadata: Asset; bytes: Uint8Array }>,
  ): Assessment {
    this.#writing();
    const source = this.revision(documentId, value.revision);
    if (
      source.metadata.sha256 !== value.sourceSha256 ||
      this.assessments(documentId, value.revision).length >= LIMITS.assessments
    )
      throw new Refusal(
        "assessment_capacity",
        "source identity differs or retained assessment capacity is exhausted",
      );
    for (const asset of assets)
      this.addAsset(documentId, value.revision, asset.metadata, asset.bytes);
    schemas.assessment.parse(value);
    this.#db
      .prepare(
        "INSERT INTO assessments(id,document_id,revision_id,payload) VALUES(?,?,?,?)",
      )
      .run(value.id, documentId, value.revision, canonicalJSON(value));
    return value;
  }

  addAsset(
    documentId: string,
    revisionId: string,
    metadata: Asset,
    content: Uint8Array,
  ): Asset {
    this.#writing();
    schemas.asset.parse(metadata);
    if (
      metadata.size !== content.byteLength ||
      metadata.sha256 !== digest(content)
    )
      throw new Error(
        "deliverables: derivative asset bytes differ from their identity",
      );
    this.#db
      .prepare(
        "INSERT INTO assets(id,document_id,revision_id,metadata,bytes) VALUES(?,?,?,?,?)",
      )
      .run(
        metadata.id,
        documentId,
        revisionId,
        canonicalJSON(metadata),
        content,
      );
    return metadata;
  }

  asset(id: string): { metadata: Asset; bytes: Uint8Array } {
    const source = this.#db
      .prepare("SELECT document_id FROM revisions WHERE id=?")
      .get(id);
    if (source) {
      const revision = this.revision(text(source.document_id), id);
      return { metadata: revision.metadata.native, bytes: revision.bytes };
    }
    const row = this.#db
      .prepare("SELECT metadata,bytes FROM assets WHERE id=?")
      .get(id);
    if (!row)
      throw new Refusal(
        "asset_not_found",
        "deliverable asset is no longer retained",
      );
    const metadata = schemas.asset.parse(json(row.metadata));
    const content = bytes(row.bytes);
    if (
      metadata.id !== id ||
      metadata.size !== content.byteLength ||
      metadata.sha256 !== digest(content)
    )
      throw new Error("deliverables: derivative bytes or identity are corrupt");
    return { metadata, bytes: content };
  }

  chunk(
    id: string,
    offset: number,
    length: number,
  ): { metadata: Asset; bytes: Uint8Array } {
    const rows = this.#db
      .prepare(
        "SELECT json_extract(metadata,'$.native') AS metadata,length(bytes) AS size,substr(bytes,?,?) AS bytes FROM revisions WHERE id=? UNION ALL SELECT metadata,length(bytes) AS size,substr(bytes,?,?) AS bytes FROM assets WHERE id=?",
      )
      .all(offset + 1, length, id, offset + 1, length, id);
    if (!rows.length)
      throw new Refusal(
        "asset_not_found",
        "deliverable asset is no longer retained",
      );
    if (rows.length !== 1)
      throw new Error("deliverables: asset identity has competing byte owners");
    const row = rows[0]!;
    const metadata = schemas.asset.parse(json(row.metadata));
    if (metadata.id !== id || BigInt(metadata.size) !== row.size)
      throw new Error(
        "deliverables: chunk metadata differs from its byte owner",
      );
    if (offset > metadata.size)
      throw new Refusal(
        "download_offset",
        "download offset exceeds the exact asset size",
      );
    const content = bytes(row.bytes);
    if (content.byteLength !== Math.min(length, metadata.size - offset))
      throw new Error("deliverables: immutable asset chunk is incomplete");
    return { metadata, bytes: content };
  }

  upload(id: string): {
    metadata: { id: string; sha256: string; size: number; received: number };
    bytes: Uint8Array;
  } {
    const row = this.#db.prepare("SELECT * FROM uploads WHERE id=?").get(id);
    if (!row)
      throw new Refusal(
        "upload_not_found",
        "staging upload is no longer retained",
      );
    const content = bytes(row.bytes);
    const metadata = schemas.uploaded.parse({
      id: text(row.id),
      sha256: text(row.digest),
      size: Number(row.size),
      received: content.byteLength,
    });
    if (metadata.received > metadata.size)
      throw new Error(
        "deliverables: staging upload exceeds its declared capacity",
      );
    return { metadata, bytes: content };
  }

  uploads() {
    return this.#db
      .prepare("SELECT id FROM uploads ORDER BY rowid")
      .all()
      .map((row) => this.upload(text(row.id)).metadata);
  }

  beginUpload(id: string, size: number, hash: string) {
    this.#writing();
    if (this.uploads().length >= LIMITS.uploads)
      throw new Refusal(
        "upload_capacity",
        "discard a staging upload before starting another",
      );
    this.#db
      .prepare("INSERT INTO uploads(id,size,digest,bytes) VALUES(?,?,?,?)")
      .run(id, size, hash, new Uint8Array());
    return this.upload(id).metadata;
  }

  appendUpload(id: string, offset: number, content: Uint8Array) {
    this.#writing();
    const current = this.upload(id);
    if (current.metadata.received !== offset)
      throw new Refusal(
        "upload_offset",
        "upload offset differs from its acknowledged bytes",
      );
    if (
      !content.byteLength ||
      content.byteLength > LIMITS.chunkBytes ||
      offset + content.byteLength > current.metadata.size
    )
      throw new Refusal(
        "upload_capacity",
        "chunk exceeds its declared upload capacity",
      );
    this.#db
      .prepare("UPDATE uploads SET bytes=? WHERE id=?")
      .run(Buffer.concat([current.bytes, content]), id);
    return this.upload(id).metadata;
  }

  discardUpload(id: string): boolean {
    this.#writing();
    this.upload(id);
    this.#db.prepare("DELETE FROM uploads WHERE id=?").run(id);
    return true;
  }

  remove(id: string, expected: string): boolean {
    this.#writing();
    this.document(id).requireHead(expected);
    if (
      this.#db.prepare("DELETE FROM documents WHERE id=?").run(id).changes !==
      1n
    )
      throw new Error("deliverables: document removal is unconfirmed");
    return true;
  }

  close(): void {
    if (this.#closed) return;
    this.#db.close();
    this.#closed = true;
  }
}
