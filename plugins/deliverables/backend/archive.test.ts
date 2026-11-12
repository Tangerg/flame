import { mkdtemp, rm } from "node:fs/promises";
import { DatabaseSync } from "node:sqlite";
import { LIMITS } from "../contracts/limits";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, it, vi } from "vitest";
import { Refusal, canonicalJSON, digestJSON } from "@flame/backend-sdk/mcp";
import { Archive, type Original } from "./archive";
import { Document, digest, revision } from "./document";

const cleanup: Array<() => void | Promise<void>> = [];
afterEach(async () => {
  for (const close of cleanup.splice(0).reverse()) await close();
});

async function fixture() {
  const directory = await mkdtemp(join(tmpdir(), "flame-deliverable-archive-"));
  cleanup.push(() => rm(directory, { recursive: true, force: true }));
  const path = join(directory, "deliverables.db");
  const archive = new Archive(path);
  cleanup.push(() => archive.close());
  return { archive, path };
}

it("preserves startup, rollback and close failures together", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-deliverable-startup-"));
  cleanup.push(() => rm(directory, { recursive: true, force: true }));
  const failure = new Error("schema commit failed");
  const rollback = new Error("schema rollback failed");
  const close = new Error("schema close failed");
  const execute = DatabaseSync.prototype.exec;
  let release: (() => void) | undefined;
  const injected = vi
    .spyOn(DatabaseSync.prototype, "exec")
    .mockImplementation(function (this: DatabaseSync, sql: string) {
      release ??= () => {
        if (this.isOpen) this.close();
      };
      if (sql === "COMMIT") throw failure;
      if (sql === "ROLLBACK") throw rollback;
      return execute.call(this, sql);
    });
  const closing = vi
    .spyOn(DatabaseSync.prototype, "close")
    .mockImplementationOnce(() => {
      throw close;
    });
  try {
    expect(() => new Archive(join(directory, "deliverables.db"))).toThrow(
      expect.objectContaining({ errors: [failure, rollback, close] }),
    );
  } finally {
    injected.mockRestore();
    closing.mockRestore();
    release?.();
  }
});

function original(
  id: string,
  input: unknown = {},
  method = "create",
): Original {
  return {
    id,
    kind: "command",
    method,
    input: canonicalJSON(input),
    inputDigest: digestJSON(input),
    invocation: {
      id: `invocation.${id}`,
      origin: "human",
      target: { scope: "installation", reference: "fixture" },
    },
  };
}
const signal = () => new AbortController().signal;
const provenance = {
  sources: [],
  assumptions: [],
  units: "USD",
  reproduction: "fixture",
};

function initial(command: string) {
  const bytes = Buffer.from("immutable native fixture");
  const source = revision({
    command,
    documentId: digest(`document:${command}`),
    title: "Fixture",
    format: "docx",
    operation: "create",
    createdAt: "2026-10-29T00:00:00.000Z",
    bytes,
    provenance,
  });
  return { bytes, source, owner: Document.create("Fixture", source) };
}

it("publishes native bytes and their original receipt atomically and never resurrects removed content", async () => {
  const { archive, path } = await fixture();
  const command = original("create-one");
  const file = initial(command.id);
  const prepare = () => () =>
    archive.publish(file.owner, file.source, file.bytes, { checks: [] });
  expect(await archive.command(command, signal(), prepare)).toEqual(
    file.source,
  );
  expect(
    archive.revision(file.source.documentId, file.source.id).bytes,
  ).toEqual(new Uint8Array(file.bytes));
  archive.close();
  const reopened = new Archive(path);
  cleanup.push(() => reopened.close());
  expect(
    await reopened.command(command, signal(), () => {
      throw new Error("must not execute again");
    }),
  ).toEqual(file.source);
  await reopened.command(
    original("delete-one"),
    signal(),
    () => () => reopened.remove(file.source.documentId, file.source.id),
  );
  expect(
    await reopened.command(command, signal(), () => {
      throw new Error("must not recreate removed content");
    }),
  ).toEqual(file.source);
  expect(reopened.list(undefined, 10).entries).toEqual([]);
  expect(() => reopened.asset(file.source.id)).toThrow("no longer retained");
});

it("an interrupted admission stays unconfirmed across reopening and binds its exact original caller and input", async () => {
  const { archive, path } = await fixture();
  const command = original("unknown", { a: 1 });
  await expect(
    archive.command(command, signal(), async () => {
      throw new Error("worker connection lost");
    }),
  ).rejects.toThrow("worker connection lost");
  expect(
    archive.receipt(
      {
        id: command.id,
        method: command.method,
        inputDigest: command.inputDigest,
      },
      "command",
    ),
  ).toEqual({ found: false });
  archive.close();
  const reopened = new Archive(path);
  cleanup.push(() => reopened.close());
  await expect(
    reopened.command(command, signal(), () => {
      throw new Error("redispatch");
    }),
  ).rejects.toThrow("unconfirmed");
  await expect(
    reopened.command(original("unknown", { a: 2 }), signal(), () => () => true),
  ).rejects.toMatchObject({ code: "command_conflict" });
  await expect(
    reopened.command(
      { ...command, invocation: { ...command.invocation, origin: "model" } },
      signal(),
      () => () => true,
    ),
  ).rejects.toMatchObject({ code: "command_conflict" });
});

it("rolls publication back on a known refusal and retains only that original refusal", async () => {
  const { archive } = await fixture();
  const command = original("refused");
  const file = initial(command.id);
  await expect(
    archive.command(command, signal(), () => () => {
      archive.publish(file.owner, file.source, file.bytes, {});
      throw new Refusal("inspection_failed", "source-bound check failed");
    }),
  ).rejects.toMatchObject({ code: "inspection_failed" });
  expect(archive.list(undefined, 10).entries).toEqual([]);
  expect(
    archive.receipt(
      {
        id: command.id,
        method: command.method,
        inputDigest: command.inputDigest,
      },
      "command",
    ),
  ).toEqual({
    found: true,
    reply: {
      failure: {
        code: "inspection_failed",
        message: "source-bound check failed",
      },
    },
  });
});

it("current revision compare-and-swap rejects prepared edits after another revision is committed", async () => {
  const { archive } = await fixture();
  const file = initial("first");
  await archive.command(
    original("first"),
    signal(),
    () => () => archive.publish(file.owner, file.source, file.bytes, {}),
  );
  function next(command: string) {
    const bytes = Buffer.from(command);
    const source = revision({
      command,
      documentId: file.source.documentId,
      title: "Fixture",
      format: "docx",
      parent: file.source.id,
      operation: "edit",
      createdAt: "2026-10-29T00:01:00.000Z",
      bytes,
      provenance,
    });
    const owner = file.owner.revise(file.source.id, source);
    return () => archive.publish(owner, source, bytes, {}, file.source.id);
  }
  const winner = next("winner"),
    stale = next("stale");
  await archive.command(original("winner"), signal(), () => winner);
  await expect(
    archive.command(original("stale"), signal(), () => stale),
  ).rejects.toMatchObject({ code: "revision_conflict" });
  expect(
    archive.history(file.source.documentId, undefined, 10).entries,
  ).toHaveLength(2);
  expect(
    archive.revision(file.source.documentId, file.source.id).bytes,
  ).toEqual(new Uint8Array(file.bytes));
});

it("upload offsets and identity are authoritative across original-command replay", async () => {
  const { archive } = await fixture();
  const content = Buffer.from([0, 255, 1, 128]);
  const id = digest("upload");
  await archive.command(
    original("begin"),
    signal(),
    () => () => archive.beginUpload(id, content.length, digest(content)),
  );
  const append = original("chunk", { id, offset: 0 });
  const result = await archive.command(
    append,
    signal(),
    () => () => archive.appendUpload(id, 0, content.subarray(0, 2)),
  );
  await archive.command(
    original("tail"),
    signal(),
    () => () => archive.appendUpload(id, 2, content.subarray(2)),
  );
  expect(
    await archive.command(append, signal(), () => {
      throw new Error("duplicate bytes");
    }),
  ).toEqual(result);
  await expect(
    archive.command(
      original("duplicate"),
      signal(),
      () => () => archive.appendUpload(id, 0, content),
    ),
  ).rejects.toMatchObject({ code: "upload_offset" });
  expect(archive.uploads()).toEqual([
    { id, sha256: digest(content), size: 4, received: 4 },
  ]);
  expect(archive.upload(id).bytes).toEqual(new Uint8Array(content));
});

it("cancelled preparation settles the refusal before publication", async () => {
  const { archive } = await fixture();
  const command = original("cancelled");
  const file = initial(command.id);
  const controller = new AbortController();
  await expect(
    archive.command(command, controller.signal, async () => {
      controller.abort(new Error("cancelled"));
      return () => archive.publish(file.owner, file.source, file.bytes, {});
    }),
  ).rejects.toThrow("cancelled");
  expect(archive.list(undefined, 10).entries).toEqual([]);
  expect(
    archive.receipt(
      {
        id: command.id,
        method: command.method,
        inputDigest: command.inputDigest,
      },
      "command",
    ),
  ).toMatchObject({ found: true, reply: { failure: { code: "cancelled" } } });
});

it("settled history leaves admission open and discarded upload bytes leave no copy in receipts", async () => {
  const { archive, path } = await fixture();
  archive.close();
  const database = new DatabaseSync(path);
  try {
    database
      .prepare(
        `WITH RECURSIVE history(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM history WHERE i<?)
      INSERT INTO commands(id,kind,method,digest,invocation,reply)
      SELECT 'old-'||i, 'command','fixture',?,?,'{"value":true}' FROM history`,
      )
      .run(
        LIMITS.pendingCommands,
        digestJSON({}),
        canonicalJSON(original("seed").invocation),
      );
  } finally {
    database.close();
  }
  const current = new Archive(path);
  cleanup.push(() => current.close());
  const payload = Buffer.from("a nonreplayable upload");
  const id = digest("history-upload");
  await current.command(
    original("begin-after-history"),
    signal(),
    () => () => current.beginUpload(id, payload.length, digest(payload)),
  );
  const append = original("append-after-history", {
    base64: payload.toString("base64"),
  });
  const evidence = await current.command(
    append,
    signal(),
    () => () => current.appendUpload(id, 0, payload),
  );
  await current.command(
    original("discard-after-history"),
    signal(),
    () => () => current.discardUpload(id),
    "reclaim",
  );
  expect(current.uploads()).toEqual([]);
  current.close();
  const recovered = new Archive(path);
  cleanup.push(() => recovered.close());
  expect(
    await recovered.command(append, signal(), () => {
      throw new Error("must not recreate discarded bytes");
    }),
  ).toEqual(evidence);
});
