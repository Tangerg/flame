import { mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { CommandStorage } from "./commandStore";

const directories: string[] = [];
const endpoint = "http://127.0.0.1:17171";
function directory(): string {
  const value = mkdtempSync(join(tmpdir(), "flame-ide-"));
  directories.push(value);
  return value;
}
afterEach(() => {
  for (const value of directories.splice(0)) rmSync(value, { recursive: true, force: true });
});

describe("IDE command storage", () => {
  it("publishes opaque SDK records atomically and keeps independent owners' records", () => {
    const root = directory();
    const first = new CommandStorage(root, endpoint);
    const second = new CommandStorage(root, endpoint);
    first.set("first", { value: "exact source" });
    second.set("second", { value: "another command" });
    expect(new Set(first.keys())).toEqual(new Set(["first", "second"]));
    expect(() => second.set("first", { value: "replacement" })).toThrow(/EEXIST/);
    expect(first.get("first")).toEqual({ value: "exact source" });
    second.remove("first");
    first.remove("first");
    expect(second.keys()).toEqual(["second"]);
    expect(new CommandStorage(root, "https://other.example").keys()).toEqual([]);
  });

  it("encodes keys without granting filesystem traversal", () => {
    const storage = new CommandStorage(directory(), endpoint);
    const key = "../remote/command:你好";
    storage.set(key, { input: "immutable" });
    expect(storage.keys()).toEqual([key]);
    expect(storage.get(key)).toEqual({ input: "immutable" });
    storage.remove(key);
    expect(storage.keys()).toEqual([]);
  });

  it("refuses damaged records and the replaced IDE-owned journal format", () => {
    const root = directory();
    const storage = new CommandStorage(root, endpoint);
    storage.set("current", { record: true });
    const target = join(root, readdirSync(root)[0]!);
    writeFileSync(join(target, readdirSync(target)[0]!), "{");
    expect(() => storage.get("current")).toThrow(SyntaxError);
    storage.remove("current");
    writeFileSync(join(target, "00000000-0000-0000-0000-000000000000.json"), "{}");
    expect(() => storage.keys()).toThrow("resolve saved commands before upgrading");
  });
});
