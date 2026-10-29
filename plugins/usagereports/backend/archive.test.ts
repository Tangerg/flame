import { afterEach, expect, it } from "vitest";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";
import {
  Capability,
  Refusal,
  canonicalJSON,
  digestJSON,
  parseJSON,
  type CapabilityDescriptor,
} from "@qimu/node";
import sample from "@flame/runtime-client/testing/samples/usageSummary.json";
import { Archive } from "./archive";
import { LIMITS } from "../contracts/limits";

const contract = new Capability(
  parseJSON(
    readFileSync(
      new URL("../contracts/capability.json", import.meta.url),
      "utf8",
    ),
  ) as CapabilityDescriptor,
);
const capture = contract.method("flame.usage-reports/capture.v1").descriptor
  .name;
const resources: Array<() => void> = [];
afterEach(() => {
  for (const close of resources.splice(0).reverse()) close();
});
function fixture() {
  const directory = mkdtempSync(join(tmpdir(), "flame-reports-"));
  resources.push(() => rmSync(directory, { recursive: true, force: true }));
  const path = join(directory, "reports.db");
  const open = () => {
    const archive = new Archive(path, contract);
    resources.push(() => archive.close());
    return archive;
  };
  return { path, open };
}
function command(id: string, input: unknown = { period: "daily" }) {
  return {
    commandId: id,
    requestInput: canonicalJSON(input),
    inputDigest: digestJSON(input),
    signal: new AbortController().signal,
  };
}
function accounting() {
  return {
    ...sample,
    observedAt: "2026-10-20T00:00:00.000000001Z",
    since: "2026-10-19T00:00:00Z",
  };
}
function receipt(
  archive: Archive,
  id: string,
  input: unknown = { period: "daily" },
) {
  return archive.receipt({
    id,
    method: capture,
    inputDigest: digestJSON(input),
  });
}

it("keeps exact accounting, immutable report identities and original receipts through close/removal", async () => {
  const f = fixture();
  let archive = f.open();
  let reads = 0;
  const raw = canonicalJSON({
    ...accounting(),
    total: {
      inputTokens: 9007199254740993n,
      costUsd: parseJSON("1.0000000000000001"),
    },
  });
  const read = async (days: number) => {
    reads++;
    expect(days).toBe(1);
    return raw;
  };
  const first = await archive.capture(command("first"), read);
  expect(canonicalJSON(archive.read(first.id).summary)).toBe(raw);
  expect(archive.list()).toEqual([first]);
  archive.close();
  archive = f.open();
  expect(await archive.capture(command("first"), read)).toEqual(first);
  expect(reads).toBe(1);
  expect(archive.remove(command("remove", { id: first.id }))).toBe(true);
  expect(await archive.capture(command("first"), read)).toEqual(first);
  expect(archive.list()).toEqual([]);
  expect(() => archive.read(first.id)).toThrow(Refusal);
  const next = await archive.capture(command("next"), read);
  expect(next.id).not.toBe(first.id);
  expect(archive.remove(command("remove", { id: first.id }))).toBe(true);
  expect(archive.list()).toEqual([next]);
  expect(receipt(archive, "first")).toEqual({
    found: true,
    reply: { value: first },
  });
  await expect(
    archive.capture(command("first", { period: "weekly" }), read),
  ).rejects.toMatchObject({ code: "command_conflict" });
  expect(reads).toBe(2);
});

it("never observes Core again after a lost callback or a lost publication", async () => {
  for (const window of ["callback", "publication"]) {
    const f = fixture();
    let archive = f.open();
    let reads = 0;
    const read = async () => {
      reads++;
      if (window === "callback") throw new Error("lost core response");
      archive.close();
      return canonicalJSON(accounting());
    };
    await expect(archive.capture(command("unknown"), read)).rejects.toThrow();
    archive.close();
    archive = f.open();
    expect(receipt(archive, "unknown")).toEqual({ found: false });
    await expect(archive.capture(command("unknown"), read)).rejects.toThrow(
      "unconfirmed",
    );
    expect(reads).toBe(1);
    expect(archive.list()).toEqual([]);
  }
});

it("rolls back publication and settlement together when a real SQLite constraint fails", async () => {
  const f = fixture();
  let archive = f.open();
  archive.close();
  const value = accounting();
  const id = digestJSON({
    Command: "collision",
    Period: "daily",
    Value: value,
  });
  const external = new DatabaseSync(f.path);
  external
    .prepare("INSERT INTO reports(id,snapshot) VALUES(?,?)")
    .run(id, "{}");
  external.close();
  archive = f.open();
  let reads = 0;
  const read = async () => {
    reads++;
    return canonicalJSON(value);
  };
  await expect(archive.capture(command("collision"), read)).rejects.toThrow();
  expect(receipt(archive, "collision")).toEqual({ found: false });
  await expect(archive.capture(command("collision"), read)).rejects.toThrow(
    "unconfirmed",
  );
  expect(reads).toBe(1);
  expect(() => archive.list()).toThrow("invalid stored evidence");
});

it("settles bounded capacity refusals without truncating or changing their original result", async () => {
  const { open } = fixture();
  const archive = open();
  let reads = 0;
  const read = async () => {
    reads++;
    return canonicalJSON(accounting());
  };
  for (let index = 0; index < LIMITS.maxReports; index++)
    await archive.capture(command("saved-" + index), read);
  await expect(archive.capture(command("full"), read)).rejects.toMatchObject({
    code: "archive_capacity",
  });
  const first = archive.list()[0]!;
  archive.remove(command("space", { id: first.id }));
  await expect(archive.capture(command("full"), read)).rejects.toMatchObject({
    code: "archive_capacity",
  });
  expect(reads).toBe(LIMITS.maxReports + 1);
  const huge = {
    ...accounting(),
    byProvider: [
      { ...sample.byProvider[0]!, key: "x".repeat(LIMITS.maxReportBytes) },
    ],
  };
  await expect(
    archive.capture(command("too-large"), async () => canonicalJSON(huge)),
  ).rejects.toMatchObject({ code: "invalid_report" });
  expect(receipt(archive, "too-large")).toMatchObject({
    found: true,
    reply: { failure: { code: "invalid_report" } },
  });
  expect(archive.list()).toHaveLength(LIMITS.maxReports - 1);
});

it("rejects bad time/range evidence and keeps its admission unconfirmed", async () => {
  const archive = fixture().open();
  for (const [index, value] of [
    { ...accounting(), observedAt: "2026-02-30T00:00:00Z" },
    { ...accounting(), since: "2026-10-20T00:00:00.000000002Z" },
    { ...accounting(), observedAt: "0001-01-01T00:00:00Z" },
    { ...accounting(), since: undefined },
  ].entries()) {
    await expect(
      archive.capture(command("invalid-" + index), async () =>
        canonicalJSON(value),
      ),
    ).rejects.toThrow();
    expect(receipt(archive, "invalid-" + index)).toEqual({ found: false });
  }
});

it("rejects a second writer and an incompatible schema without rewriting old data", () => {
  const f = fixture();
  const archive = f.open();
  expect(() => new Archive(f.path, contract)).toThrow(/locked/);
  archive.close();
  const old = new DatabaseSync(f.path);
  old.exec("CREATE TABLE old_owner(value TEXT)");
  old.close();
  const before = readFileSync(f.path);
  expect(() => new Archive(f.path, contract)).toThrow(
    "fresh private data directory",
  );
  expect(readFileSync(f.path).equals(before)).toBe(true);
});

it("exposes corrupted snapshots and outcomes instead of reporting an empty archive", async () => {
  for (const column of ["snapshot", "reply", "input"]) {
    const f = fixture();
    let archive = f.open();
    const saved = await archive.capture(command("saved"), async () =>
      canonicalJSON(accounting()),
    );
    archive.close();
    const db = new DatabaseSync(f.path);
    if (column === "snapshot")
      db.prepare("UPDATE reports SET snapshot='{}' WHERE id=?").run(saved.id);
    else if (column === "reply")
      db.exec("UPDATE commands SET reply='{\"value\":false}'");
    else db.exec('UPDATE commands SET input=\'{"period":"weekly"}\'');
    db.close();
    archive = f.open();
    expect(() => archive.list()).toThrow();
    expect(() => archive.remove(command("remove", { id: saved.id }))).toThrow();
    expect(
      archive.receipt({
        id: "remove",
        method: contract.method("flame.usage-reports/delete.v1").descriptor
          .name,
        inputDigest: digestJSON({ id: saved.id }),
      }),
    ).toEqual({ found: false });
  }
});
