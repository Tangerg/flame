import { mkdtempSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { DatabaseSync } from "node:sqlite";
import { afterEach, describe, expect, it } from "vitest";
import { canonicalJSON, digestJSON } from "@qimu/node";
import { Receipts, type OriginalCommand, type ReceiptLimits } from "./receipts";

const limits: ReceiptLimits = {
  maxRecords: 100,
  maxBytes: 1 << 20,
  maxInputBytes: 4096,
  maxResultBytes: 16384,
};
const directories: string[] = [];
const stores: Receipts[] = [];

afterEach(() => {
  for (const store of stores.splice(0)) store.close();
  for (const directory of directories.splice(0))
    rmSync(directory, { recursive: true, force: true });
});

function database(overrides: Partial<ReceiptLimits> = {}) {
  const directory = mkdtempSync(join(tmpdir(), "flame-computer-receipts-"));
  directories.push(directory);
  const path = join(directory, "computer.db");
  const budget = { ...limits, ...overrides };
  const open = () => {
    const store = new Receipts(path, budget);
    stores.push(store);
    return store;
  };
  return { path, open, store: open() };
}

function original(
  id = "original",
  kind: OriginalCommand["kind"] = "command",
  value: unknown = { code: "await cua.getState()" },
): OriginalCommand {
  return {
    id: kind === "tool" ? `tool:${id}` : id,
    kind,
    method: kind === "tool" ? "js" : "flame.computer/open.v1",
    input: canonicalJSON(value),
    inputDigest: digestJSON(value),
    invocation: {
      id,
      origin: kind === "tool" ? "model" : "human",
      target: { scope: "installation", reference: "plg_original" },
    },
    admittedAt: "2026-10-28T00:00:00.000Z",
  };
}

function request(command: OriginalCommand) {
  return {
    id: command.id,
    method: command.method,
    inputDigest: command.inputDigest,
  };
}

describe("original computer receipts", () => {
  it("durably admits before dispatch and never readmits an original action", () => {
    const { store, open } = database();
    const command = original();
    expect(store.admit(command)).toEqual({ fresh: true });
    expect(
      store.admit({ ...command, admittedAt: "2026-10-28T01:00:00.000Z" }),
    ).toEqual({
      fresh: false,
      record: { ...command, dispatched: false, state: "pending" },
    });
    store.markDispatched(command.id);
    expect(() => store.markDispatched(command.id)).toThrow(/again/);
    store.close();
    const restarted = open();
    expect(restarted.admit(command)).toEqual({
      fresh: false,
      record: { ...command, dispatched: true, state: "pending" },
    });
    expect(restarted.receipt(request(command))).toEqual({ found: false });
  });

  it("retains an observed outcome independently of execution cancellation", () => {
    const { store, open } = database();
    const command = original();
    const signal = new AbortController();
    store.admit(command);
    store.markDispatched(command.id);
    signal.abort();
    store.settleCommand(command.id, { value: { session: "control_original" } });
    store.close();
    const restarted = open();
    expect(restarted.receipt(request(command))).toEqual({
      found: true,
      reply: { value: { session: "control_original" } },
    });
    expect(restarted.admit(command).fresh).toBe(false);
    expect(() => restarted.markDispatched(command.id)).toThrow();
  });

  it("captures one immutable result and derives tool recovery without a second result", () => {
    const { store, open } = database();
    const command = original("tool-owner", "tool");
    store.admit(command);
    store.markDispatched(command.id);
    const result = {
      content: [
        {
          type: "image" as const,
          mimeType: "image/png",
          data: "AQID",
          _meta: { "codex/imageDetail": "original" },
        },
        { type: "text" as const, text: "observed" },
      ],
      structuredContent: { count: 1 },
    };
    store.settleTool(command.id, result);
    result.content[1]!.text = "changed";
    result.structuredContent.count = 2;
    const captured = store.read(command.id);
    expect(captured).toMatchObject({
      outcome: { structuredContent: { count: 1 } },
    });
    const recovered = store.toolReceipt(request(command));
    expect(recovered).toEqual({
      found: true,
      output: canonicalJSON({
        content: [
          {
            kind: "media",
            media: {
              mime: "image/png",
              source: { kind: "bytes", bytes: "AQID" },
            },
            metadata: {
              "mcp/content": {
                type: "image",
                _meta: { "codex/imageDetail": "original" },
              },
            },
          },
          { kind: "text", text: "observed" },
        ],
        details: { count: 1 },
      }),
    });
    store.close();
    expect(open().toolReceipt(request(command))).toEqual(recovered);
  });

  it("settles known pre-dispatch refusals without inventing a dispatch", () => {
    const { store } = database();
    const command = original();
    store.admit(command);
    const reply = {
      failure: {
        code: "lease_expired",
        message: "computer control lease expired",
      },
    };
    store.settleCommand(command.id, reply);
    store.settleCommand(command.id, reply);
    expect(store.read(command.id)).toMatchObject({
      dispatched: false,
      state: "settled",
      outcome: reply,
    });
    expect(() => store.settleCommand(command.id, { value: true })).toThrow(
      /different evidence/,
    );
  });

  it("preserves the original failure media and structured content", () => {
    const { store } = database();
    const command = original("failed-tool", "tool");
    store.admit(command);
    store.settleTool(command.id, {
      isError: true,
      content: [{ type: "text", text: "permission declined" }],
      structuredContent: { status: "declined" },
    });
    expect(store.toolReceipt(request(command))).toMatchObject({
      found: true,
      isError: true,
    });
    expect(store.read(command.id)).toMatchObject({
      dispatched: false,
      state: "settled",
    });
  });

  it.each([
    [
      "method",
      (command: OriginalCommand) => ({ ...command, method: "another.method" }),
    ],
    [
      "input",
      (command: OriginalCommand) => ({
        ...command,
        input: canonicalJSON({ code: "other" }),
        inputDigest: digestJSON({ code: "other" }),
      }),
    ],
    [
      "origin",
      (command: OriginalCommand) => ({
        ...command,
        invocation: { ...command.invocation, origin: "model" as const },
      }),
    ],
    [
      "target",
      (command: OriginalCommand) => ({
        ...command,
        invocation: {
          ...command.invocation,
          target: { scope: "installation", reference: "plg_other" },
        },
      }),
    ],
    [
      "parent",
      (command: OriginalCommand) => ({
        ...command,
        invocation: { ...command.invocation, parent: "different-parent" },
      }),
    ],
  ])("refuses a second %s under one original identity", (_name, alter) => {
    const { store } = database();
    const command = original();
    store.admit(command);
    expect(() => store.admit(alter(command))).toThrow(/different evidence/);
    expect(store.read(command.id)).toEqual({
      ...command,
      dispatched: false,
      state: "pending",
    });
  });

  it("refuses receipt reads that substitute the original method, digest or kind", () => {
    const { store } = database();
    const command = original();
    store.admit(command);
    store.settleCommand(command.id, { value: true });
    expect(() =>
      store.receipt({ ...request(command), method: "foreign" }),
    ).toThrow();
    expect(() =>
      store.receipt({ ...request(command), inputDigest: "f".repeat(64) }),
    ).toThrow();
    expect(() => store.toolReceipt(request(command))).toThrow();
    expect(store.receipt({ ...request(command), id: "missing" })).toEqual({
      found: false,
    });
  });

  it("does not settle unadmitted commands or change command output kinds", () => {
    const { store } = database();
    expect(() => store.settleCommand("missing", { value: true })).toThrow();
    const command = original();
    store.admit(command);
    expect(() => store.settleTool(command.id, { content: [] })).toThrow();
    expect(() =>
      store.settleCommand(command.id, {
        value: true,
        failure: { code: "bad", message: "" },
      }),
    ).toThrow();
    expect(store.receipt(request(command))).toEqual({ found: false });
  });

  it("reserves bounded outcome space before admitting an external action", () => {
    const { store } = database({
      maxBytes: 5500,
      maxResultBytes: 2000,
      maxInputBytes: 500,
    });
    const first = original("first");
    const second = original("second");
    store.admit(first);
    expect(() => store.admit(second)).toThrow(/capacity/);
    expect(store.read(first.id)).toMatchObject({ state: "pending" });
    expect(store.read(second.id)).toBeUndefined();
    store.settleCommand(first.id, { value: true });
    expect(store.admit(second)).toEqual({ fresh: true });
  });

  it("never prunes pending or settled originals to accept a newer command", () => {
    const { store, open } = database({ maxRecords: 2 });
    const command = original();
    store.admit(command);
    store.settleCommand(command.id, { value: true });
    expect(() => store.admit(original("newer"))).toThrow(/capacity/);
    store.close();
    const restarted = open();
    expect(restarted.receipt(request(command)).found).toBe(true);
    expect(() => restarted.admit(original("after-restart"))).toThrow(
      /capacity/,
    );
  });

  it("preserves one revocation admission when ordinary history reaches its record limit", () => {
    const { store, open } = database({ maxRecords: 2 });
    const first = original("first");
    const revocation = original("revocation");
    store.admit(first);
    store.markDispatched(first.id);
    expect(() => store.admit(revocation)).toThrow(/capacity/);
    expect(store.admit(revocation, "reserved")).toEqual({ fresh: true });
    store.settleCommand(revocation.id, { value: { closed: true } });
    for (const admission of ["ordinary", "reserved"] as const) {
      expect(store.admit(first, admission)).toMatchObject({
        fresh: false,
        record: { state: "pending", dispatched: true },
      });
      expect(store.admit(revocation, admission)).toMatchObject({
        fresh: false,
        record: { state: "settled" },
      });
      expect(() => store.admit(original("third"), admission)).toThrow(
        /capacity/,
      );
    }
    store.close();
    const restarted = open();
    expect(restarted.admit(first)).toMatchObject({
      fresh: false,
      record: { state: "pending", dispatched: true },
    });
    expect(restarted.receipt(request(revocation))).toEqual({
      found: true,
      reply: { value: { closed: true } },
    });
    expect(() => restarted.admit(original("newer"), "reserved")).toThrow(
      /capacity/,
    );
  });

  it("reserves complete input and outcome bytes for revocation while an action stays unknown", () => {
    const { store, open } = database({
      maxBytes: 2048,
      maxInputBytes: 512,
      maxResultBytes: 512,
    });
    const first = original("first", "command", {});
    const revocation = original("revoke", "command", {
      reason: "界".repeat(70),
    });
    store.admit(first);
    store.markDispatched(first.id);
    expect(() => store.admit(original("next"))).toThrow(/capacity/);
    store.close();
    const restarted = open();
    expect(restarted.admit(revocation, "reserved")).toEqual({ fresh: true });
    const result = { value: "x".repeat(500) };
    expect(Buffer.byteLength(canonicalJSON(result))).toBe(512);
    restarted.settleCommand(revocation.id, result);
    expect(restarted.receipt(request(revocation))).toEqual({
      found: true,
      reply: result,
    });
    expect(restarted.read(first.id)).toMatchObject({
      state: "pending",
      dispatched: true,
    });
    expect(() => restarted.admit(original("third"), "reserved")).toThrow(
      /capacity/,
    );
    expect(restarted.list({ limit: 10 }).records).toHaveLength(2);
  });

  it("bounds UTF-8 input and result bytes without truncating retained evidence", () => {
    const { store } = database({ maxInputBytes: 512, maxResultBytes: 128 });
    expect(() =>
      store.admit(original("large", "command", { text: "界".repeat(171) })),
    ).toThrow(/input capacity/);
    const command = original("bounded", "command", {});
    store.admit(command);
    expect(() =>
      store.settleCommand(command.id, { value: "界".repeat(43) }),
    ).toThrow(/capacity/);
    expect(store.read(command.id)).toMatchObject({ state: "pending" });
  });

  it("bounds captured invocation and identity inside the same original evidence budget", () => {
    const { store } = database({ maxInputBytes: 512 });
    const command = original();
    expect(() =>
      store.admit({
        ...command,
        invocation: {
          ...command.invocation,
          target: { scope: "installation", reference: "界".repeat(171) },
        },
      }),
    ).toThrow(/input capacity/);
    expect(() =>
      store.admit({
        ...command,
        id: "x".repeat(256),
        method: "m".repeat(256),
      }),
    ).toThrow(/input capacity/);
    expect(store.list({ limit: 10 }).records).toEqual([]);
  });

  it.each([
    { maxRecords: 1 },
    { maxBytes: 1024, maxInputBytes: 512, maxResultBytes: 512 },
    { maxBytes: 1024, maxInputBytes: 600, maxResultBytes: 600 },
  ])("rejects budgets that cannot reserve a whole command: %j", (budget) => {
    expect(() => database(budget)).toThrow(/budgets/);
  });

  it("pages immutable summaries and never exposes large payloads in the list", () => {
    const { store } = database();
    for (const id of ["first", "second", "third"]) store.admit(original(id));
    const page = store.list({ limit: 2 });
    expect(page.records.map((record) => record.id)).toEqual([
      "third",
      "second",
    ]);
    expect(page.nextCursor).toBe("second");
    expect(page.records[0]).not.toHaveProperty("input");
    expect(
      store
        .list({ before: page.nextCursor, limit: 2 })
        .records.map((record) => record.id),
    ).toEqual(["first"]);
    expect(() => store.list({ before: "missing", limit: 2 })).toThrow();
    expect(() => store.list({ limit: 0 })).toThrow();
  });

  it("holds one private database writer and preserves data after a refused second owner", () => {
    const { path, store, open } = database();
    store.admit(original());
    expect(() => new Receipts(path, limits)).toThrow();
    expect(store.read("original")).toBeDefined();
    store.close();
    expect(open().read("original")).toBeDefined();
  });

  it("refuses incompatible private data without replacing it", () => {
    const { path, store } = database();
    store.close();
    const db = new DatabaseSync(path);
    db.exec(
      "CREATE TABLE foreign_data (value TEXT); INSERT INTO foreign_data VALUES('preserve')",
    );
    db.close();
    expect(() => new Receipts(path, limits)).toThrow(/incompatible/);
    const inspect = new DatabaseSync(path);
    expect(inspect.prepare("SELECT value FROM foreign_data").get()?.value).toBe(
      "preserve",
    );
    inspect.close();
  });

  it("detects corrupt input evidence and closes refused startup acquisitions", () => {
    const { path, store, open } = database();
    store.admit(original());
    store.close();
    const db = new DatabaseSync(path);
    db.prepare("UPDATE commands SET input=?").run('{"code":"tampered"}');
    db.close();
    expect(() => open().read("original")).toThrow(/digest/);
  });

  it("requires exact trusted invocation and a canonical admitted request", () => {
    const { store } = database();
    const command = original();
    expect(() =>
      store.admit({ ...command, input: '{ "code": "await cua.getState()" }' }),
    ).toThrow();
    expect(() =>
      store.admit({ ...command, invocation: undefined as never }),
    ).toThrow();
    expect(() =>
      store.admit({ ...command, admittedAt: "2026-02-30T00:00:00.000Z" }),
    ).toThrow();
    expect(store.list({ limit: 10 }).records).toEqual([]);
  });
});
