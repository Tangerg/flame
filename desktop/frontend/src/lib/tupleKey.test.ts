import { describe, expect, it } from "vitest";
import { tupleKey } from "./tupleKey";

describe("tupleKey", () => {
  it("preserves field boundaries even when values contain control characters", () => {
    expect(tupleKey("a\u0000b", "c")).not.toBe(tupleKey("a", "b\u0000c"));
  });

  it("preserves empty fields, order, and Unicode exactly", () => {
    expect(tupleKey("", "Flame 中文")).not.toBe(tupleKey("Flame 中文", ""));
    expect(tupleKey("scope", "Flame 中文")).toBe(tupleKey("scope", "Flame 中文"));
  });
});
