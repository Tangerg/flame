import { mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { authorizeCommand, CommandStore, type Command } from "./commandStore";

const directories: string[] = [];
const endpoint = "http://127.0.0.1:17171";
const scope = { namespace: "idp_store", retentionSeconds: 3600 };
const command: Command = {
  kind: "start",
  params: { sessionId: "ses_1", input: [{ type: "text", text: "exact unsaved input" }] },
};
function directory(): string {
  const value = mkdtempSync(join(tmpdir(), "flame-ide-"));
  directories.push(value);
  return value;
}
afterEach(() => {
  vi.useRealTimers();
  for (const value of directories.splice(0)) rmSync(value, { recursive: true, force: true });
});

describe("prepared IDE command ownership", () => {
  it("retains original command bytes and identity independently of later editor mutations", () => {
    const root = directory();
    const store = new CommandStore(root, endpoint);
    const input = structuredClone(command);
    const pending = store.prepare(scope.namespace, scope.retentionSeconds, input);
    input.params = { sessionId: "different", input: [{ type: "text", text: "changed" }] };
    const saved = new CommandStore(root, endpoint).read(pending.id)!;
    expect(saved.id).toBe(pending.id);
    expect(saved.command).toEqual(command);
  });

  it("cannot overwrite another extension host's command or settle its successor", () => {
    const root = directory();
    const first = new CommandStore(root, endpoint);
    const second = new CommandStore(root, endpoint);
    const pendingFirst = first.prepare(scope.namespace, scope.retentionSeconds, command);
    const pendingSecond = second.prepare(scope.namespace, scope.retentionSeconds, command);
    expect(new Set(first.list().map((pending) => pending.id))).toEqual(
      new Set([pendingFirst.id, pendingSecond.id]),
    );
    first.settle(pendingFirst.id);
    second.settle(pendingFirst.id);
    expect(second.list().map((pending) => pending.id)).toEqual([pendingSecond.id]);
    expect(new CommandStore(root, "https://other.example").list()).toEqual([]);
  });

  it("refuses namespace substitution and does not extend retention after restart", () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000_000);
    const root = directory();
    const store = new CommandStore(root, endpoint);
    const pending = store.prepare(scope.namespace, 60, command);
    expect(() => authorizeCommand(pending, "idp_replacement")).toThrow("different Runtime store");
    vi.advanceTimersByTime(60_000);
    const saved = new CommandStore(root, endpoint).read(pending.id)!;
    expect(() => authorizeCommand(saved, scope.namespace)).toThrow("replay retention");
  });

  it("fails closed on corrupted saved parameters and refuses path-shaped identities", () => {
    const root = directory();
    const store = new CommandStore(root, endpoint);
    const pending = store.prepare(scope.namespace, scope.retentionSeconds, command);
    const target = join(root, readdirSync(root)[0]!, `${pending.id}.json`);
    writeFileSync(
      target,
      JSON.stringify({ ...pending, command: { kind: "start", params: { input: [] } } }),
    );
    expect(() => new CommandStore(root, endpoint).read(pending.id)).toThrow(
      "invalid pending IDE command parameters",
    );
    expect(() => store.settle("../other-file")).toThrow("invalid IDE command identity");
  });
});
